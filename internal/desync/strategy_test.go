package desync

import (
	"bytes"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/UmutKDev/gecit/internal/tlsx"
)

func hello(t *testing.T, host string) []byte {
	c, s := net.Pipe()
	go func() {
		cl := tls.Client(c, &tls.Config{ServerName: host})
		cl.SetDeadline(time.Now().Add(time.Second))
		cl.Handshake()
	}()
	rec, err := tlsx.ReadRecord(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	return rec
}

func TestPlanTLSNoSegmentContainsFullSNI(t *testing.T) {
	rec := hello(t, "discord.com")
	for _, st := range []Strategy{
		{Split: SplitSNI},
		{RecordSplit: true},
		{RecordSplit: true, Split: SplitSNI},
		{Split: SplitChunk, ChunkSize: 3},
	} {
		p := st.PlanTLS(rec)
		if p.Host != "discord.com" {
			t.Fatalf("%+v: host %q", st, p.Host)
		}
		for _, seg := range p.Segments() {
			if bytes.Contains(seg, []byte("discord.com")) {
				t.Fatalf("%+v: bir parça SNI'nin tamamını içeriyor", st)
			}
		}
	}
}

func TestPlanTLSPassthroughKeepsBytes(t *testing.T) {
	rec := hello(t, "example.org")
	p := Strategy{Split: SplitSNI}.PlanTLS(rec)
	if !bytes.Equal(bytes.Join(p.Segments(), nil), rec) {
		t.Fatal("yalnız TCP bölmesi baytları değiştirmemeli")
	}
}

func TestPlanHTTP(t *testing.T) {
	req := []byte("GET / HTTP/1.1\r\nHost: www.wattpad.com\r\nAccept: */*\r\n\r\n")
	p := Strategy{HostCase: true, HostNoSpace: true, HostMixedValue: true, HTTPSplit: true}.PlanHTTP(req)
	s := string(p.Data)
	if !strings.Contains(s, "hoSt:wWwWaTtPaD.CoM") && !strings.Contains(s, "hoSt:") {
		t.Fatalf("beklenmeyen istek: %q", s)
	}
	if strings.Contains(s, "Host: www.wattpad.com") {
		t.Fatal("Host başlığı değişmedi")
	}
	if len(p.Segments()) != 2 {
		t.Fatalf("parça sayısı %d", len(p.Segments()))
	}
	if p.Host != "www.wattpad.com" {
		t.Fatalf("host %q", p.Host)
	}
}
