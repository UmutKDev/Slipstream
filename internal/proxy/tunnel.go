package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/UmutKDev/Slipstream/internal/desync"
	"github.com/UmutKDev/Slipstream/internal/tlsx"
)

// httpMethods, düz HTTP isteğini tanımak için kullanılan başlangıçlardır.
var httpMethods = [][]byte{
	[]byte("GET "), []byte("POST "), []byte("HEAD "), []byte("PUT "),
	[]byte("DELETE "), []byte("OPTIONS "), []byte("PATCH "),
}

// blockMarkers, ISS uyarı sayfalarında geçen izlerdir. Türkiye'de engelli
// HTTP siteleri 5651 sayılı kanuna atıfla uyarı sayfasına yönlendirilir.
var blockMarkers = [][]byte{[]byte("195.175.254."), []byte("5651")}

// maxAttempts, bir bağlantı için denenecek en fazla strateji sayısıdır.
const maxAttempts = 4

type firstKind int

const (
	kindRaw firstKind = iota
	kindTLS
	kindHTTP
)

// first, istemcinin ilk paketidir.
type first struct {
	kind firstKind
	data []byte
	host string
	// verifiable, sunucunun bu paketten hemen sonra yanıt vermesi
	// bekleniyorsa true olur. Gövdeli HTTP isteklerinde sunucu gövdeyi
	// beklediği için doğrulama yapılmaz.
	verifiable bool
}

// tunnel, istemcinin ilk paketini stratejiye göre dönüştürüp yollar, sonra
// iki yönlü kopyalamaya geçer. Otomatik öğrenme açıksa ve sunucu yanıt
// vermezse, istemci hiçbir şey fark etmeden yeni bir bağlantıyla sıradaki
// strateji denenir. Çalışan strateji o site için hatırlanır.
func (e *Engine) tunnel(client net.Conn, br *bufio.Reader, up net.Conn, host string, port int) {
	client.SetReadDeadline(time.Now().Add(15 * time.Second))
	fp, err := readFirst(br, host)
	client.SetReadDeadline(time.Time{})
	if err != nil {
		up.Close()
		return
	}

	if fp.kind == kindRaw || !e.Domains.Apply(fp.host) {
		if len(fp.data) > 0 {
			if _, err := up.Write(fp.data); err != nil {
				up.Close()
				return
			}
		}
		e.pipe(client, br, up, nil)
		return
	}

	chain := e.chainFor(fp.host)
	if !e.AutoLearn() || !fp.verifiable {
		chain = chain[:1]
	}
	for i, st := range chain {
		if i > 0 {
			up, err = e.Dial(context.Background(), host, port)
			if err != nil {
				break
			}
		}
		p := plan(st, fp)
		if err := desync.Send(up, p, st); err != nil {
			up.Close()
			e.Log.Warnf("%s: gönderilemedi (%s): %v", fp.host, st.Name, err)
			continue
		}
		e.Stats.Bypassed.Add(1)
		if len(chain) == 1 {
			e.Log.Infof("%s %s → %s", kindName(fp.kind), fp.host, st.Name)
			e.pipe(client, br, up, nil)
			return
		}
		resp, err := awaitResponse(up, e.ProbeTimeout)
		if err == nil {
			if i > 0 || !e.isLearned(fp.host, st) {
				e.learn(fp.host, st)
			}
			if i > 0 {
				e.Log.Infof("%s %s → %s (%d. denemede çalıştı, kaydedildi)", kindName(fp.kind), fp.host, st.Name, i+1)
			} else {
				e.Log.Infof("%s %s → %s", kindName(fp.kind), fp.host, st.Name)
			}
			e.pipe(client, br, up, resp)
			return
		}
		up.Close()
		e.Log.Warnf("%s: %s stratejisi çalışmadı (%v)", fp.host, st.Name, err)
	}
	e.Stats.Failed.Add(1)
	e.forget(fp.host)
	e.Log.Errorf("%s: hiçbir strateji çalışmadı", fp.host)
}

func kindName(k firstKind) string {
	if k == kindHTTP {
		return "HTTP"
	}
	return "TLS"
}

func plan(st desync.Strategy, fp first) desync.Plan {
	if fp.kind == kindHTTP {
		return st.PlanHTTP(fp.data)
	}
	return st.PlanTLS(fp.data)
}

// readFirst, istemcinin ilk paketini türüyle birlikte okur.
func readFirst(br *bufio.Reader, host string) (first, error) {
	b, err := br.Peek(1)
	if err != nil || len(b) == 0 {
		return first{kind: kindRaw}, nil
	}
	if b3, _ := br.Peek(3); tlsx.IsTLSHandshake(b3) {
		rec, err := tlsx.ReadRecord(br)
		if errors.Is(err, tlsx.ErrNotClientHello) {
			return first{kind: kindRaw, data: rec}, nil
		}
		if err != nil {
			return first{}, err
		}
		fp := first{kind: kindTLS, data: rec, host: host, verifiable: true}
		if info, err := tlsx.ParseSNI(rec); err == nil {
			fp.host = info.Host
		}
		return fp, nil
	}
	if isHTTP(br) {
		req := readHTTPHead(br)
		fp := first{kind: kindHTTP, data: req, host: host}
		head := strings.ToLower(string(req))
		hasBody := strings.Contains(head, "\r\ncontent-length:") && !strings.Contains(head, "\r\ncontent-length: 0\r\n") ||
			strings.Contains(head, "\r\ntransfer-encoding:")
		fp.verifiable = !hasBody
		return fp, nil
	}
	return first{kind: kindRaw}, nil
}

// awaitResponse, sunucunun ilk yanıtını bekler. Bağlantı sıfırlanırsa, kapanırsa,
// süre dolarsa ya da yanıt bir ISS uyarı sayfasıysa hata döner.
func awaitResponse(up net.Conn, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	up.SetReadDeadline(time.Now().Add(timeout))
	defer up.SetReadDeadline(time.Time{})
	buf := make([]byte, 16<<10)
	n, err := up.Read(buf)
	if n == 0 {
		if err == nil || err == io.EOF {
			err = errors.New("sunucu yanıt vermeden kapattı")
		}
		return nil, err
	}
	resp := buf[:n]
	if bytes.HasPrefix(resp, []byte("HTTP/")) {
		for _, m := range blockMarkers {
			if bytes.Contains(resp, m) {
				return nil, errors.New("ISS uyarı sayfası döndü")
			}
		}
	}
	return resp, nil
}

// pipe, iki yönlü kopyalamayı yürütür. pending, istemciye önce yazılacak
// sunucu verisidir.
func (e *Engine) pipe(client net.Conn, br *bufio.Reader, up net.Conn, pending []byte) {
	defer up.Close()
	if len(pending) > 0 {
		if _, err := client.Write(pending); err != nil {
			return
		}
		e.Stats.BytesDn.Add(int64(len(pending)))
	}
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(up, br)
		e.Stats.BytesUp.Add(n)
		if tc, ok := up.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(client, up)
		e.Stats.BytesDn.Add(n)
		if tc, ok := client.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	// Bir yön kapandıktan sonra öbür yöne kısa bir süre tanı.
	t := time.NewTimer(5 * time.Second)
	select {
	case <-done:
	case <-t.C:
	}
	t.Stop()
}

func isHTTP(br *bufio.Reader) bool {
	b, _ := br.Peek(8)
	for _, m := range httpMethods {
		if bytes.HasPrefix(b, m) {
			return true
		}
	}
	return false
}

// readHTTPHead, istek başlıklarını (en fazla 16 KB) okur. Başlık sonu
// gelmezse o ana kadar okunanı döndürür.
func readHTTPHead(br *bufio.Reader) []byte {
	var buf []byte
	for len(buf) < 16<<10 {
		line, err := br.ReadSlice('\n')
		buf = append(buf, line...)
		if err != nil || bytes.Equal(line, []byte("\r\n")) || bytes.Equal(line, []byte("\n")) {
			break
		}
	}
	return buf
}
