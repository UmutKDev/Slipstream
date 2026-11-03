package dns

import (
	"context"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestParseAnswer(t *testing.T) {
	name := dnsmessage.MustNewName("discord.com.")
	m := dnsmessage.Message{
		Header: dnsmessage.Header{Response: true},
		Answers: []dnsmessage.Resource{
			{Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 300},
				Body: &dnsmessage.AResource{A: [4]byte{162, 159, 128, 233}}},
			{Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120},
				Body: &dnsmessage.AResource{A: [4]byte{162, 159, 135, 232}}},
		},
	}
	wire, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	ips, ttl, err := ParseAnswer(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 || ips[0].String() != "162.159.128.233" || ttl != 120 {
		t.Fatalf("ips=%v ttl=%d", ips, ttl)
	}
}

func TestLookupIPLiteralAndCache(t *testing.T) {
	r, _ := New("cloudflare")
	ips, err := r.Lookup(context.Background(), "1.2.3.4")
	if err != nil || ips[0].String() != "1.2.3.4" {
		t.Fatal("IP doğrudan dönmeli")
	}
	r.cache["ornek.test"] = entry{ips, time.Now().Add(time.Minute)}
	got, err := r.Lookup(context.Background(), "ornek.test")
	if err != nil || got[0].String() != "1.2.3.4" {
		t.Fatal("önbellek kullanılmadı")
	}
}

func TestUnknownProvider(t *testing.T) {
	if _, err := New("yok"); err == nil {
		t.Fatal("hata bekleniyordu")
	}
}
