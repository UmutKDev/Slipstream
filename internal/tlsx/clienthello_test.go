package tlsx

import (
	"bytes"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

// captureHello, crypto/tls'in ürettiği gerçek bir ClientHello kaydını yakalar.
func captureHello(t *testing.T, host string) []byte {
	t.Helper()
	c, s := net.Pipe()
	go func() {
		cl := tls.Client(c, &tls.Config{ServerName: host})
		cl.SetDeadline(time.Now().Add(time.Second))
		cl.Handshake()
	}()
	rec, err := ReadRecord(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	return rec
}

func TestParseSNI(t *testing.T) {
	rec := captureHello(t, "discord.com")
	info, err := ParseSNI(rec)
	if err != nil {
		t.Fatal(err)
	}
	if info.Host != "discord.com" {
		t.Fatalf("host = %q", info.Host)
	}
	if string(rec[info.Offset:info.Offset+info.Length]) != "discord.com" {
		t.Fatal("offset yanlış")
	}
}

func TestSplitRecordReassembles(t *testing.T) {
	rec := captureHello(t, "www.wattpad.com")
	info, _ := ParseSNI(rec)
	cut := info.Offset - RecordHeaderLen + info.Length/2
	parts := SplitRecord(rec, 1, cut)
	if len(parts) != 3 {
		t.Fatalf("parça sayısı %d", len(parts))
	}
	var payload []byte
	for _, p := range parts {
		if p[0] != RecordTypeHandshake {
			t.Fatal("kayıt türü bozuldu")
		}
		payload = append(payload, p[RecordHeaderLen:]...)
	}
	if !bytes.Equal(payload, rec[RecordHeaderLen:]) {
		t.Fatal("yük yeniden birleşmedi")
	}
}

func TestParseSNIRejectsGarbage(t *testing.T) {
	if _, err := ParseSNI([]byte("GET / HTTP/1.1\r\n")); err == nil {
		t.Fatal("hata bekleniyordu")
	}
	if _, err := ParseSNI([]byte{0x16, 3, 1, 0, 10, 1, 0, 0}); err == nil {
		t.Fatal("hata bekleniyordu")
	}
}
