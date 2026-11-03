package tunnel

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/UmutKDev/Slipstream/internal/adblock"
	"github.com/UmutKDev/Slipstream/internal/dns"
	"golang.org/x/net/dns/dnsmessage"
)

func query(t *testing.T, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 0xbeef, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET}},
	}
	b, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPacketRoundTrip(t *testing.T) {
	src, dst := [4]byte{198, 18, 0, 1}, [4]byte{198, 18, 0, 2}
	pkt := BuildUDP4(src, dst, 53000, 53, []byte("merhaba"))
	p, err := ParseUDP4(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if p.Src != src || p.Dst != dst || p.SrcPort != 53000 || p.DstPort != 53 || string(p.Payload) != "merhaba" {
		t.Fatalf("yanlış çözüldü: %+v", p)
	}
	if checksum(pkt[:20], 0) != 0 {
		t.Error("IP sağlama toplamı tutmuyor")
	}
	r, err := ParseUDP4(p.Reply([]byte("selam")))
	if err != nil || r.Src != dst || r.DstPort != 53000 || string(r.Payload) != "selam" {
		t.Fatalf("yanıt yanlış: %+v %v", r, err)
	}
	if _, err := ParseUDP4(pkt[:20]); err == nil {
		t.Error("kısa paket kabul edildi")
	}
	tcp := append([]byte(nil), pkt...)
	tcp[9] = 6
	if _, err := ParseUDP4(tcp); err == nil {
		t.Error("TCP paketi kabul edildi")
	}
}

func TestBlockedAnswer(t *testing.T) {
	ads := adblock.New(t.TempDir())
	ads.SetLists([]string{"video-ads"})
	ads.SetEnabled(true)
	d := &DNS{Ads: ads, Resolver: func() *dns.Resolver { t.Fatal("engelli ad çözücüye gitmemeli"); return nil }}
	resp, err := d.Answer(context.Background(), query(t, "imasdk.googleapis.com.", dnsmessage.TypeA))
	if err != nil {
		t.Fatal(err)
	}
	var m dnsmessage.Message
	if err := m.Unpack(resp); err != nil {
		t.Fatal(err)
	}
	if m.ID != 0xbeef || !m.Response || len(m.Answers) != 1 {
		t.Fatalf("yanıt beklenmedik: %+v", m)
	}
	if a := m.Answers[0].Body.(*dnsmessage.AResource); net.IP(a.A[:]).String() != "0.0.0.0" {
		t.Errorf("adres %v", a.A)
	}
	if _, err := d.Answer(context.Background(), []byte{1, 2, 3}); err == nil {
		t.Error("bozuk sorgu kabul edildi")
	}
}

func TestForwardAndCache(t *testing.T) {
	// Sahte bir UDP DNS sunucusu: her sorguya 1.2.3.4 döner.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	var hits atomic.Int32
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			hits.Add(1)
			var m dnsmessage.Message
			m.Unpack(buf[:n])
			m.Response = true
			m.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: m.Questions[0].Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120},
				Body:   &dnsmessage.AResource{A: [4]byte{1, 2, 3, 4}},
			}}
			out, _ := m.Pack()
			pc.WriteTo(out, addr)
		}
	}()
	old := dns.FallbackUDP
	dns.FallbackUDP = pc.LocalAddr().String()
	defer func() { dns.FallbackUDP = old }()
	res, _ := dns.New("system")
	d := &DNS{Resolver: func() *dns.Resolver { return res }}
	for i := 0; i < 2; i++ {
		resp, err := d.Answer(context.Background(), query(t, "ornek.com.", dnsmessage.TypeA))
		if err != nil {
			t.Fatal(err)
		}
		ips, _, err := dns.ParseAnswer(resp)
		if err != nil || len(ips) != 1 || ips[0].String() != "1.2.3.4" || resp[0] != 0xbe {
			t.Fatalf("yanıt %v %v", ips, err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("önbellek çalışmadı, %d sorgu gitti", hits.Load())
	}
}
