package desync

import (
	"errors"
	"net"
	"syscall"
)

// oobByte, akışa eklenen ve sunucunun atacağı fazladan bayttır.
var oobByte = []byte{'a'}

// sendOOB, seg'i normal yazar, ardından fazladan bir baytı MSG_OOB ile
// gönderir. Bu bayt acil veri olarak işaretlenir. SO_OOBINLINE kapalı olan
// sunucu onu normal akıştan çıkarır, akışı birleştiren DPI ise alan adının
// ortasında görür.
func sendOOB(c *net.TCPConn, seg []byte) error {
	if _, err := c.Write(seg); err != nil {
		return err
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var sendErr error
	err = raw.Write(func(fd uintptr) bool {
		e := syscall.Sendto(int(fd), oobByte, syscall.MSG_OOB, nil)
		if errors.Is(e, syscall.EAGAIN) {
			return false
		}
		sendErr = e
		return true
	})
	if err != nil {
		return err
	}
	return sendErr
}
