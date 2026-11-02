package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
)

// serveSOCKS, kimlik doğrulamasız SOCKS5 CONNECT isteklerini karşılar (RFC 1928).
func (e *Engine) serveSOCKS(c net.Conn, br *bufio.Reader) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(br, hdr); err != nil || hdr[0] != 5 {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(br, methods); err != nil {
		return
	}
	ok := false
	for _, m := range methods {
		if m == 0 {
			ok = true
		}
	}
	if !ok {
		c.Write([]byte{5, 0xff})
		return
	}
	c.Write([]byte{5, 0})

	req := make([]byte, 4)
	if _, err := io.ReadFull(br, req); err != nil || req[0] != 5 {
		return
	}
	if req[1] != 1 { // yalnız CONNECT
		socksReply(c, 7)
		return
	}
	var host string
	switch req[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3:
		l, err := br.ReadByte()
		if err != nil {
			return
		}
		b := make([]byte, l)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = string(b)
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		socksReply(c, 8)
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(br, pb); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(pb))
	up, err := e.Dial(context.Background(), host, port)
	if err != nil {
		e.Stats.Failed.Add(1)
		e.Log.Warnf("%v", err)
		socksReply(c, 5)
		return
	}
	if err := socksReply(c, 0); err != nil {
		up.Close()
		return
	}
	e.tunnel(c, br, up, host)
}

func socksReply(c net.Conn, code byte) error {
	_, err := c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}
