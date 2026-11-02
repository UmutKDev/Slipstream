// Package desync, DPI cihazlarını şaşırtmak için bağlantının ilk paketini
// (TLS ClientHello ya da HTTP isteği) dönüştürüp parça parça gönderir.
package desync

import (
	"bytes"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/UmutKDev/slipstream/internal/tlsx"
)

// SplitMode, TCP katmanında ilk paketin nasıl bölüneceğini belirler.
type SplitMode string

const (
	SplitNone  SplitMode = "none"
	SplitSNI   SplitMode = "sni"   // SNI ad alanının içinden böl
	SplitFixed SplitMode = "fixed" // sabit bayt konumundan böl
	SplitChunk SplitMode = "chunk" // eşit küçük parçalara böl
)

// Strategy, tek bir atlatma reçetesidir. ISS profilleri bunlardan oluşur.
type Strategy struct {
	Name string `json:"name"`

	// TLS
	RecordSplit bool      `json:"recordSplit"` // ClientHello'yu SNI ortasından iki TLS kaydına böl
	Split       SplitMode `json:"split"`
	SplitPos    int       `json:"splitPos,omitempty"`
	ChunkSize   int       `json:"chunkSize,omitempty"`
	DelayMs     int       `json:"delayMs,omitempty"` // parçalar arası bekleme

	// HTTP (80. port)
	HostCase       bool `json:"hostCase"`       // "Host:" başlığını "hoSt:" yap
	HostNoSpace    bool `json:"hostNoSpace"`    // "Host: a" yerine "Host:a"
	HostMixedValue bool `json:"hostMixedValue"` // alan adının harflerini karıştır
	HTTPSplit      bool `json:"httpSplit"`      // isteği Host değerinin ortasından böl
}

// Delay, parçalar arası bekleme süresidir.
func (s Strategy) Delay() time.Duration { return time.Duration(s.DelayMs) * time.Millisecond }

// Plan, gönderilecek baytlar ve TCP segment sınırlarıdır.
type Plan struct {
	Data []byte
	Cuts []int
	Host string
}

// PlanTLS, bir ClientHello kaydı için gönderim planı üretir.
func (s Strategy) PlanTLS(rec []byte) Plan {
	info, err := tlsx.ParseSNI(rec)
	if err != nil {
		return s.genericPlan(rec)
	}
	data := rec
	var cuts []int
	if s.RecordSplit && info.Length >= 2 {
		mid := info.Offset - tlsx.RecordHeaderLen + info.Length/2
		parts := tlsx.SplitRecord(rec, mid)
		data = bytes.Join(parts, nil)
		if len(parts) == 2 {
			cuts = append(cuts, len(parts[0]))
		}
	}
	switch s.Split {
	case SplitSNI:
		cuts = append(cuts, info.Offset+1)
		if info.Length > 4 && !s.RecordSplit {
			cuts = append(cuts, info.Offset+info.Length/2)
		}
	case SplitFixed, SplitChunk:
		cuts = append(cuts, s.genericCuts(len(data))...)
	}
	return Plan{Data: data, Cuts: normalize(cuts, len(data)), Host: info.Host}
}

func (s Strategy) genericPlan(b []byte) Plan {
	return Plan{Data: b, Cuts: normalize(s.genericCuts(len(b)), len(b))}
}

func (s Strategy) genericCuts(n int) []int {
	switch s.Split {
	case SplitFixed:
		pos := s.SplitPos
		if pos <= 0 {
			pos = 2
		}
		return []int{pos}
	case SplitChunk:
		size := s.ChunkSize
		if size <= 0 {
			size = 4
		}
		var c []int
		// Yalnız ilk 512 bayt parçalanır; gerisi tek seferde gider.
		for i := size; i < n && i <= 512; i += size {
			c = append(c, i)
		}
		return c
	}
	return nil
}

// PlanHTTP, düz HTTP isteğini dönüştürür ve bölme noktalarını hesaplar.
func (s Strategy) PlanHTTP(req []byte) Plan {
	hdrEnd := bytes.Index(req, []byte("\r\n\r\n"))
	if hdrEnd < 0 {
		return s.genericPlan(req)
	}
	head := string(req[:hdrEnd])
	lines := strings.Split(head, "\r\n")
	host := ""
	hostLine := -1
	for i, l := range lines {
		if len(l) > 5 && strings.EqualFold(l[:5], "host:") {
			host = strings.TrimSpace(l[5:])
			hostLine = i
			break
		}
	}
	if hostLine < 0 {
		return s.genericPlan(req)
	}
	key := "Host:"
	if s.HostCase {
		key = "hoSt:"
	}
	val := host
	if s.HostMixedValue {
		val = mixCase(host)
	}
	sep := " "
	if s.HostNoSpace {
		sep = ""
	}
	lines[hostLine] = key + sep + val
	newHead := strings.Join(lines, "\r\n")
	data := append([]byte(newHead), req[hdrEnd:]...)
	var cuts []int
	if s.HTTPSplit {
		off := strings.Index(newHead, key+sep+val) + len(key) + len(sep)
		cuts = append(cuts, off+len(val)/2)
	}
	if s.Split == SplitFixed || s.Split == SplitChunk {
		cuts = append(cuts, s.genericCuts(len(data))...)
	}
	return Plan{Data: data, Cuts: normalize(cuts, len(data)), Host: host}
}

func mixCase(h string) string {
	b := []byte(h)
	up := false
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			if up {
				b[i] = c - 32
			}
			up = !up
		}
	}
	return string(b)
}

func normalize(cuts []int, n int) []int {
	sort.Ints(cuts)
	out := cuts[:0]
	prev := 0
	for _, c := range cuts {
		if c > prev && c < n {
			out = append(out, c)
			prev = c
		}
	}
	return out
}

// Segments, planı gönderilecek parçalara ayırır.
func (p Plan) Segments() [][]byte {
	var segs [][]byte
	prev := 0
	for _, c := range p.Cuts {
		segs = append(segs, p.Data[prev:c])
		prev = c
	}
	return append(segs, p.Data[prev:])
}

// Send, planı conn üzerinden parça parça yazar. Her parçanın ayrı bir TCP
// segmenti olarak çıkması için Nagle algoritması kapatılır.
func Send(conn net.Conn, p Plan, delay time.Duration) error {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	segs := p.Segments()
	for i, seg := range segs {
		if _, err := conn.Write(seg); err != nil {
			return err
		}
		if i < len(segs)-1 && delay > 0 {
			time.Sleep(delay)
		}
	}
	return nil
}
