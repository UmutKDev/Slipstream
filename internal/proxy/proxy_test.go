package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/UmutKDev/slipstream/internal/desync"
	"github.com/UmutKDev/slipstream/internal/dns"
	"github.com/UmutKDev/slipstream/internal/domains"
)

// fakeDPI, Türkiye'deki SNI filtrelerini taklit eden bir ara kutudur.
// İlk okunan TCP verisinde yasaklı adı bütün olarak görürse bağlantıyı keser.
// Düz HTTP'de "Host: <yasaklı>" görürse uyarı sayfasına yönlendirir.
func fakeDPI(t *testing.T, backend string, banned string) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 64<<10)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				first := buf[:n]
				if bytes.Contains(first, []byte(banned)) {
					if bytes.Contains(first, []byte("Host: "+banned)) {
						fmt.Fprint(c, "HTTP/1.1 302 Found\r\nLocation: http://uyari.example/\r\nContent-Length: 0\r\n\r\n")
					}
					return // TLS için bağlantıyı sıfırla
				}
				b, err := net.Dial("tcp", backend)
				if err != nil {
					return
				}
				defer b.Close()
				b.Write(first)
				// Linux'ta TCPConn'lar arası io.Copy splice kullanır ve splice
				// TCP acil verisinde takılır. Gerçek bir DPI pasif dinlediği için
				// bu sorun yaşanmaz; burada düz okuma/yazmaya zorluyoruz.
				go io.Copy(b, struct{ io.Reader }{c})
				io.Copy(c, struct{ io.Reader }{b})
			}(c)
		}
	}()
	return ln.Addr().String()
}

func startEngine(t *testing.T, st desync.Strategy, banned, dpiAddr string) string {
	r, _ := dns.New("cloudflare")
	r.AddStatic(banned, net.ParseIP("127.0.0.1"))
	e := NewEngine(st, r, domains.NewMatcher(domains.ModeAll, nil))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go e.Serve(ctx, ln)
	return ln.Addr().String()
}

func httpsVia(proxyAddr, target string) (int, error) {
	u, _ := url.Parse("http://" + proxyAddr)
	cl := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(u),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := cl.Get(target)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

const banned = "engelli.example"

func TestHTTPSBypassThroughHTTPProxy(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "merhaba")
	}))
	defer srv.Close()
	dpi := fakeDPI(t, srv.Listener.Addr().String(), banned)
	_, port, _ := net.SplitHostPort(dpi)
	target := "https://" + banned + ":" + port + "/"

	// Atlatma yokken DPI bağlantıyı kesmeli.
	plain := startEngine(t, desync.Strategy{Name: "yok", Split: desync.SplitNone}, banned, dpi)
	if _, err := httpsVia(plain, target); err == nil {
		t.Fatal("atlatma olmadan istek geçmemeliydi")
	}

	for _, st := range []desync.Strategy{
		{Name: "sni", Split: desync.SplitSNI, DelayMs: 20},
		{Name: "kayit", RecordSplit: true, DelayMs: 20},
		{Name: "parca", RecordSplit: true, Split: desync.SplitChunk, ChunkSize: 8, DelayMs: 5},
		{Name: "midsld+oob", Split: desync.SplitMidSLD, OOB: true, DelayMs: 20},
	} {
		addr := startEngine(t, st, banned, dpi)
		code, err := httpsVia(addr, target)
		if err != nil || code != 200 {
			t.Fatalf("%s: code=%d err=%v", st.Name, code, err)
		}
	}
}

func TestHTTPBypass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "icerik:"+r.Host)
	}))
	defer srv.Close()
	dpi := fakeDPI(t, srv.Listener.Addr().String(), banned)
	_, port, _ := net.SplitHostPort(dpi)
	target := "http://" + banned + ":" + port + "/"

	u := func(addr string) *http.Client {
		pu, _ := url.Parse("http://" + addr)
		return &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	plain := startEngine(t, desync.Strategy{Name: "yok"}, banned, dpi)
	resp, err := u(plain).Get(target)
	if err != nil || resp.StatusCode != 302 {
		t.Fatalf("atlatmasız 302 bekleniyordu: %v %v", resp, err)
	}
	st := desync.Strategy{Name: "http", HostCase: true, HostNoSpace: true, HostMixedValue: true, HTTPSplit: true, DelayMs: 20}
	resp, err = u(startEngine(t, st, banned, dpi)).Get(target)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "icerik:") {
		t.Fatalf("code=%d body=%q", resp.StatusCode, body)
	}
}

func TestSOCKS5Bypass(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "socks")
	}))
	defer srv.Close()
	dpi := fakeDPI(t, srv.Listener.Addr().String(), banned)
	_, port, _ := net.SplitHostPort(dpi)
	addr := startEngine(t, desync.Strategy{Name: "sni", Split: desync.SplitSNI, RecordSplit: true, DelayMs: 20}, banned, dpi)
	d, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	cl := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		Dial:            d.Dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := cl.Get("https://" + banned + ":" + port + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "socks" {
		t.Fatalf("body=%q", body)
	}
}

func TestListModeSkipsOtherDomains(t *testing.T) {
	m := domains.NewMatcher(domains.ModeList, []string{"discord.com"})
	if m.Apply("example.com") || !m.Apply("gateway.discord.com") {
		t.Fatal("liste kipi yanlış eşleşti")
	}
}
