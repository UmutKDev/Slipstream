// Package dns, DNS zehirlemesini atlatmak için DNS-over-HTTPS çözücüsü sağlar.
// ISS'lerin düz DNS yanıtlarına müdahalesi bu yolla etkisiz kalır.
package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Provider, bir DoH sunucusudur. Bootstrap adresleri sayesinde sunucunun
// kendi adını çözmek için düz DNS'e ihtiyaç kalmaz.
type Provider struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Bootstrap []string `json:"bootstrap"`
}

// Providers, hazır DoH sağlayıcılarıdır.
var Providers = map[string]Provider{
	"cloudflare": {"cloudflare", "Cloudflare", "https://cloudflare-dns.com/dns-query", []string{"1.1.1.1", "1.0.0.1"}},
	"google":     {"google", "Google", "https://dns.google/dns-query", []string{"8.8.8.8", "8.8.4.4"}},
	"quad9":      {"quad9", "Quad9", "https://dns.quad9.net/dns-query", []string{"9.9.9.9", "149.112.112.112"}},
	"adguard":    {"adguard", "AdGuard", "https://dns.adguard-dns.com/dns-query", []string{"94.140.14.14", "94.140.15.15"}},
}

type entry struct {
	ips     []net.IP
	expires time.Time
}

// Resolver, önbellekli ve yedekli bir DoH çözücüsüdür.
type Resolver struct {
	providers []Provider
	clients   []*http.Client
	mu        sync.RWMutex
	cache     map[string]entry
	// System true ise DoH kullanılmaz, işletim sisteminin çözücüsüne gidilir.
	System bool
}

// New, verilen sağlayıcı kimlikleriyle bir çözücü kurar. İlk sağlayıcı
// başarısız olursa sıradakine geçilir.
func New(ids ...string) (*Resolver, error) {
	r := &Resolver{cache: map[string]entry{}}
	for _, id := range ids {
		if id == "system" {
			r.System = true
			continue
		}
		p, ok := Providers[id]
		if !ok {
			return nil, fmt.Errorf("dns: bilinmeyen sağlayıcı %q", id)
		}
		r.providers = append(r.providers, p)
		r.clients = append(r.clients, newClient(p))
	}
	if len(r.providers) == 0 {
		r.System = true
	}
	return r, nil
}

func newClient(p Provider) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var last error
			for _, ip := range p.Bootstrap {
				c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, "443"))
				if err == nil {
					return c, nil
				}
				last = err
			}
			return nil, last
		},
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 6 * time.Second}
}

// Lookup, bir ana bilgisayar adını IP adreslerine çözer. IPv4 adresleri önce gelir.
func (r *Resolver) Lookup(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	r.mu.RLock()
	e, ok := r.cache[host]
	r.mu.RUnlock()
	if ok && time.Now().Before(e.expires) {
		return e.ips, nil
	}
	if r.System && len(r.providers) == 0 {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}
	var errs []error
	for i := range r.providers {
		ips, ttl, err := r.query(ctx, i, host)
		if err == nil && len(ips) > 0 {
			r.mu.Lock()
			r.cache[host] = entry{ips, time.Now().Add(ttl)}
			r.mu.Unlock()
			return ips, nil
		}
		if err == nil {
			err = fmt.Errorf("dns: %s için kayıt yok", host)
		}
		errs = append(errs, fmt.Errorf("%s: %w", r.providers[i].Name, err))
	}
	if r.System {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}
	return nil, errors.Join(errs...)
}

// Flush, önbelleği temizler.
func (r *Resolver) Flush() {
	r.mu.Lock()
	r.cache = map[string]entry{}
	r.mu.Unlock()
}

func (r *Resolver) query(ctx context.Context, i int, host string) ([]net.IP, time.Duration, error) {
	type res struct {
		ips []net.IP
		ttl uint32
		err error
	}
	ch := make(chan res, 2)
	for _, t := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		go func(t dnsmessage.Type) {
			ips, ttl, err := r.exchange(ctx, i, host, t)
			ch <- res{ips, ttl, err}
		}(t)
	}
	var v4, v6 []net.IP
	var minTTL uint32 = 3600
	var errs []error
	for range 2 {
		x := <-ch
		if x.err != nil {
			errs = append(errs, x.err)
			continue
		}
		for _, ip := range x.ips {
			if ip.To4() != nil {
				v4 = append(v4, ip)
			} else {
				v6 = append(v6, ip)
			}
		}
		if len(x.ips) > 0 && x.ttl < minTTL {
			minTTL = x.ttl
		}
	}
	if len(errs) == 2 {
		return nil, 0, errors.Join(errs...)
	}
	if minTTL < 60 {
		minTTL = 60
	}
	return append(v4, v6...), time.Duration(minTTL) * time.Second, nil
}

func (r *Resolver) exchange(ctx context.Context, i int, host string, t dnsmessage.Type) ([]net.IP, uint32, error) {
	name, err := dnsmessage.NewName(dnsFQDN(host))
	if err != nil {
		return nil, 0, err
	}
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 0, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: name, Type: t, Class: dnsmessage.ClassINET}},
	}
	wire, err := msg.Pack()
	if err != nil {
		return nil, 0, err
	}
	body, err := r.post(ctx, i, wire)
	if err != nil {
		return nil, 0, err
	}
	return ParseAnswer(body)
}

// post, DNS iletisini i. sağlayıcıya DoH ile gönderir ve ham yanıtı döndürür.
func (r *Resolver) post(ctx context.Context, i int, wire []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.providers[i].URL, bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := r.clients[i].Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dns: %s HTTP %d", r.providers[i].Name, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

// FallbackUDP, DoH sağlayıcısı yoksa ham sorguların gönderildiği adrestir.
var FallbackUDP = "1.1.1.1:53"

// Exchange, ham bir DNS sorgusunu sırayla sağlayıcılara iletir ve ham
// yanıtı döndürür. iOS tünelinde cihazın bütün DNS sorguları buradan geçer.
// DoH önbelleğe uygun olsun diye sorgu kimliği sıfırlanır, yanıtta geri
// yazılır.
func (r *Resolver) Exchange(ctx context.Context, query []byte) ([]byte, error) {
	if len(query) < 12 {
		return nil, errors.New("dns: sorgu çok kısa")
	}
	id0, id1 := query[0], query[1]
	wire := append([]byte(nil), query...)
	wire[0], wire[1] = 0, 0
	var errs []error
	for i := range r.providers {
		resp, err := r.post(ctx, i, wire)
		if err == nil && len(resp) >= 12 {
			resp[0], resp[1] = id0, id1
			return resp, nil
		}
		if err == nil {
			err = errors.New("dns: yanıt çok kısa")
		}
		errs = append(errs, fmt.Errorf("%s: %w", r.providers[i].Name, err))
	}
	if r.System || len(r.providers) == 0 {
		resp, err := exchangeUDP(ctx, query)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

func exchangeUDP(ctx context.Context, query []byte) ([]byte, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "udp", FallbackUDP)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := c.Write(query); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// ParseAnswer, DNS yanıtındaki A/AAAA kayıtlarını ve en küçük TTL'yi döndürür.
func ParseAnswer(wire []byte) ([]net.IP, uint32, error) {
	var m dnsmessage.Message
	if err := m.Unpack(wire); err != nil {
		return nil, 0, err
	}
	if m.RCode != dnsmessage.RCodeSuccess {
		return nil, 0, fmt.Errorf("dns: yanıt kodu %v", m.RCode)
	}
	var ips []net.IP
	var ttl uint32 = 3600
	for _, a := range m.Answers {
		switch b := a.Body.(type) {
		case *dnsmessage.AResource:
			ips = append(ips, net.IP(b.A[:]))
		case *dnsmessage.AAAAResource:
			ips = append(ips, net.IP(b.AAAA[:]))
		default:
			continue
		}
		if a.Header.TTL < ttl {
			ttl = a.Header.TTL
		}
	}
	return ips, ttl, nil
}

func dnsFQDN(h string) string {
	if len(h) > 0 && h[len(h)-1] == '.' {
		return h
	}
	return h + "."
}

// AddStatic, bir ana bilgisayar adını kalıcı olarak verilen adreslere bağlar.
// Testlerde ve kullanıcı tanımlı "hosts" kayıtlarında kullanılır.
func (r *Resolver) AddStatic(host string, ips ...net.IP) {
	r.mu.Lock()
	r.cache[host] = entry{ips, time.Now().AddDate(100, 0, 0)}
	r.mu.Unlock()
}
