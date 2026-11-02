package proxy

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"time"

	"github.com/UmutKDev/gecit/internal/desync"
	"github.com/UmutKDev/gecit/internal/tlsx"
)

// httpMethods, düz HTTP isteğini tanımak için kullanılan başlangıçlardır.
var httpMethods = [][]byte{
	[]byte("GET "), []byte("POST "), []byte("HEAD "), []byte("PUT "),
	[]byte("DELETE "), []byte("OPTIONS "), []byte("PATCH "),
}

// tunnel, istemciden gelen ilk paketi stratejiye göre dönüştürüp yollar,
// sonra iki yönlü kopyalamaya geçer.
func (e *Engine) tunnel(client net.Conn, br *bufio.Reader, up net.Conn, host string) {
	defer up.Close()
	client.SetReadDeadline(time.Now().Add(15 * time.Second))
	st := e.Strategy()

	head, err := br.Peek(1)
	if err == nil && len(head) > 0 {
		if err := e.sendFirst(br, up, host, st); err != nil {
			e.Stats.Failed.Add(1)
			e.Log.Warnf("%s: ilk paket gönderilemedi: %v", host, err)
			return
		}
	}
	client.SetReadDeadline(time.Time{})

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

func (e *Engine) sendFirst(br *bufio.Reader, up net.Conn, host string, st desync.Strategy) error {
	b, _ := br.Peek(3)
	switch {
	case tlsx.IsTLSHandshake(b):
		rec, err := tlsx.ReadRecord(br)
		if err != nil && err != tlsx.ErrNotClientHello {
			return err
		}
		if err == tlsx.ErrNotClientHello {
			_, err := up.Write(rec)
			return err
		}
		p := st.PlanTLS(rec)
		if p.Host != "" {
			host = p.Host
		}
		if !e.Domains.Apply(host) {
			_, err := up.Write(rec)
			return err
		}
		e.Stats.Bypassed.Add(1)
		e.Log.Infof("TLS %s → %s (%d parça)", host, st.Name, len(p.Cuts)+1)
		return desync.Send(up, p, st.Delay())
	case isHTTP(br):
		req := readHTTPHead(br)
		if !e.Domains.Apply(host) {
			_, err := up.Write(req)
			return err
		}
		p := st.PlanHTTP(req)
		e.Stats.Bypassed.Add(1)
		e.Log.Infof("HTTP %s → %s", host, st.Name)
		return desync.Send(up, p, st.Delay())
	}
	return nil
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
