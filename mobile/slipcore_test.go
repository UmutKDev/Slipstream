package slipcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/UmutKDev/Slipstream/internal/tunnel"
	"golang.org/x/net/dns/dnsmessage"
)

type fakeDelegate struct {
	pkts    chan []byte
	proxies chan string
}

func (f *fakeDelegate) WritePacket(p []byte)  { f.pkts <- p }
func (f *fakeDelegate) ProxyChanged(s string) { f.proxies <- s }

func TestStartRequestAndDNS(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"profile":"dengeli","dns":["cloudflare"],"mode":"all","listen":"127.0.0.1:0",
	"systemProxy":true,"proxyMode":"global","adBlock":true,"adLists":["video-ads"]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &fakeDelegate{pkts: make(chan []byte, 4), proxies: make(chan string, 4)}
	out, err := Start(dir, `{"profile":"turktelekom"}`, d)
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()
	var ps proxySettings
	if err := json.Unmarshal([]byte(out), &ps); err != nil || !ps.Enabled || ps.Port == 0 || ps.Mode != "global" {
		t.Fatalf("proxy ayarı %s %v", out, err)
	}

	var resp response
	json.Unmarshal([]byte(Request("GET", "/api/status", "")), &resp)
	if resp.Status != 200 {
		t.Fatalf("durum %d %s", resp.Status, resp.Body)
	}
	var st struct{ Profile string }
	json.Unmarshal([]byte(resp.Body), &st)
	if st.Profile != "turktelekom" {
		t.Errorf("profil seçeneği uygulanmadı: %s", st.Profile)
	}

	// PAC kipine geçince Swift'e yeni ayar gider.
	json.Unmarshal([]byte(Request("POST", "/api/systemproxy", `{"enabled":true,"mode":"pac"}`)), &resp)
	if resp.Status != 200 {
		t.Fatalf("systemproxy %d %s", resp.Status, resp.Body)
	}
	select {
	case s := <-d.proxies:
		json.Unmarshal([]byte(s), &ps)
		if ps.Mode != "pac" || ps.PAC == "" {
			t.Errorf("PAC ayarı eksik: %s", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProxyChanged çağrılmadı")
	}

	// Engelli alan adı için DNS paketi yanıtlanır.
	msg := dnsmessage.Message{Header: dnsmessage.Header{ID: 7, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName("imasdk.googleapis.com."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}
	q, _ := msg.Pack()
	HandlePacket(tunnel.BuildUDP4([4]byte{198, 18, 0, 1}, [4]byte{198, 18, 0, 2}, 5353, 53, q))
	select {
	case p := <-d.pkts:
		u, err := tunnel.ParseUDP4(p)
		if err != nil || u.DstPort != 5353 || u.SrcPort != 53 {
			t.Fatalf("yanıt paketi yanlış: %+v %v", u, err)
		}
		var m dnsmessage.Message
		if err := m.Unpack(u.Payload); err != nil || m.ID != 7 || len(m.Answers) != 1 {
			t.Fatalf("DNS yanıtı yanlış: %+v %v", m, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DNS yanıtı gelmedi")
	}

	Stop()
	if Running() {
		t.Error("durmadı")
	}
	json.Unmarshal([]byte(Request("GET", "/api/status", "")), &resp)
	if resp.Status != 503 {
		t.Errorf("kapalıyken %d", resp.Status)
	}
}
