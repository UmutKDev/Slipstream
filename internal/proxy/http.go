package proxy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// serveHTTP, HTTP proxy isteklerini karşılar. CONNECT bir tünel açar,
// diğer yöntemler hedefe doğrudan iletilir.
func (e *Engine) serveHTTP(c net.Conn, br *bufio.Reader) {
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method == http.MethodConnect {
		host, port := splitHostPort(req.Host, 443)
		up, err := e.Dial(context.Background(), host, port)
		if err != nil {
			e.Stats.Failed.Add(1)
			e.Log.Warnf("%v", err)
			fmt.Fprint(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
			return
		}
		if _, err := c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			up.Close()
			return
		}
		e.tunnel(c, br, up, host)
		return
	}
	e.forwardHTTP(c, br, req)
}

// forwardHTTP, mutlak biçimli (http://...) bir isteği hedefe iletir. İstek
// yeniden yazılır, sonra tünel aynı DPI dönüşümünü uygular.
func (e *Engine) forwardHTTP(c net.Conn, br *bufio.Reader, req *http.Request) {
	if req.URL.Host == "" {
		fmt.Fprint(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		return
	}
	host, port := splitHostPort(req.URL.Host, 80)
	up, err := e.Dial(context.Background(), host, port)
	if err != nil {
		e.Stats.Failed.Add(1)
		e.Log.Warnf("%v", err)
		fmt.Fprint(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	for _, h := range []string{"Proxy-Connection", "Proxy-Authorization", "Proxy-Authenticate"} {
		req.Header.Del(h)
	}
	// Tek istek, tek bağlantı. Tarayıcı bir sonraki istekte yeniden bağlanır.
	req.Close = true
	req.Header.Set("Connection", "close")
	req.RequestURI = ""
	var buf bytes.Buffer
	if err := req.Write(&buf); err != nil {
		up.Close()
		return
	}
	// Yeniden yazılmış istek önce gider. Ardından istemciden gelen her şey
	// aynı bağlantıya akar; sunucu "Connection: close" gördüğü için kapanır.
	mr := bufio.NewReaderSize(io.MultiReader(&buf, br), 32<<10)
	e.tunnel(c, mr, up, host)
}

func splitHostPort(hp string, def int) (string, int) {
	h, p, err := net.SplitHostPort(hp)
	if err != nil {
		return strings.Trim(hp, "[]"), def
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		n = def
	}
	return h, n
}
