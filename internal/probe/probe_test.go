package probe

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/UmutKDev/gecit/internal/desync"
)

// dpi, ilk okumada "example.com" görürse bağlantıyı kesen bir ara kutudur.
func dpi(t *testing.T, backend string) string {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 65536)
				n, err := c.Read(buf)
				if err != nil || bytes.Contains(buf[:n], []byte("example.com")) {
					return
				}
				b, err := net.Dial("tcp", backend)
				if err != nil {
					return
				}
				defer b.Close()
				b.Write(buf[:n])
				go io.Copy(b, c)
				io.Copy(c, b)
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestRunFindsWorkingStrategy(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	addr := dpi(t, srv.Listener.Addr().String())
	dial := func(ctx context.Context, host string, port int) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	pool := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	rep := Run(context.Background(), dial, []desync.Strategy{
		{Name: "sni", Split: desync.SplitSNI, DelayMs: 20},
	}, Options{Targets: []string{"example.com"}, Timeout: 2 * time.Second, RootCAs: pool})
	if len(rep.Blocked) != 1 {
		t.Fatalf("engelli site bulunamadı: %+v", rep.Baseline)
	}
	if rep.Best == nil || rep.Best.Strategy.Name != "sni" || rep.Best.OK != 1 {
		t.Fatalf("en iyi strateji bulunamadı: %+v", rep.Results)
	}
}
