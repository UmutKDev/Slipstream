// Package probe, kullanıcının kendi hattında hangi stratejinin çalıştığını
// bulur. Her stratejiyle engelli alan adlarına gerçek TLS bağlantısı kurar,
// sertifikayı doğrular ve bir HTTP yanıtı bekler.
package probe

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/UmutKDev/slipstream/internal/desync"
)

// DefaultTargets, Türkiye'de engelli olduğu bilinen ve deneme için
// kullanılan alan adlarıdır.
var DefaultTargets = []string{"discord.com", "gateway.discord.gg", "www.roblox.com", "www.wattpad.com"}

// Dialer, bir alan adına TCP bağlantısı açar (DoH çözümlemesi dahil).
type Dialer func(ctx context.Context, host string, port int) (net.Conn, error)

// Result, tek bir stratejinin sonucudur.
type Result struct {
	Strategy  desync.Strategy   `json:"strategy"`
	OK        int               `json:"ok"`
	Total     int               `json:"total"`
	LatencyMs int64             `json:"latencyMs"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// Report, tüm denemenin özetidir.
type Report struct {
	Baseline Result   `json:"baseline"`
	Results  []Result `json:"results"`
	Best     *Result  `json:"best,omitempty"`
	Blocked  []string `json:"blocked"`
}

// Options, deneme ayarlarıdır.
type Options struct {
	Targets  []string
	Timeout  time.Duration
	Progress func(msg string)
	RootCAs  *x509.CertPool // boşsa sistem sertifikaları kullanılır
}

// Run, önce stratejisiz bir deneme yapar, sonra adayları sırayla dener.
func Run(ctx context.Context, dial Dialer, candidates []desync.Strategy, o Options) Report {
	if len(o.Targets) == 0 {
		o.Targets = DefaultTargets
	}
	if o.Timeout == 0 {
		o.Timeout = 6 * time.Second
	}
	say := func(f string, a ...any) {
		if o.Progress != nil {
			o.Progress(fmt.Sprintf(f, a...))
		}
	}
	var rep Report
	say("Atlatmasız bağlantı deneniyor")
	rep.Baseline = try(ctx, dial, desync.Strategy{Name: "yok", Split: desync.SplitNone}, o)
	for h := range rep.Baseline.Errors {
		rep.Blocked = append(rep.Blocked, h)
	}
	sort.Strings(rep.Blocked)
	if len(rep.Blocked) == 0 {
		say("Deneme listesindeki sitelerin hiçbiri engelli görünmüyor")
	}
	for _, st := range candidates {
		if ctx.Err() != nil {
			break
		}
		say("%s stratejisi deneniyor", st.Name)
		r := try(ctx, dial, st, o)
		rep.Results = append(rep.Results, r)
		say("%s: %d/%d başarılı", st.Name, r.OK, r.Total)
	}
	for i := range rep.Results {
		r := &rep.Results[i]
		if r.OK == 0 {
			continue
		}
		if rep.Best == nil || r.OK > rep.Best.OK || (r.OK == rep.Best.OK && r.LatencyMs < rep.Best.LatencyMs) {
			rep.Best = r
		}
	}
	return rep
}

func try(ctx context.Context, dial Dialer, st desync.Strategy, o Options) Result {
	res := Result{Strategy: st, Total: len(o.Targets), Errors: map[string]string{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var sum time.Duration
	for _, host := range o.Targets {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			d, err := once(ctx, dial, st, host, o)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				res.Errors[host] = err.Error()
				return
			}
			res.OK++
			sum += d
		}(host)
	}
	wg.Wait()
	if res.OK > 0 {
		res.LatencyMs = (sum / time.Duration(res.OK)).Milliseconds()
	}
	return res
}

func once(ctx context.Context, dial Dialer, st desync.Strategy, host string, o Options) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	start := time.Now()
	raw, err := dial(ctx, host, 443)
	if err != nil {
		return 0, err
	}
	defer raw.Close()
	deadline, _ := ctx.Deadline()
	raw.SetDeadline(deadline)
	c := tls.Client(&Conn{Conn: raw, Strategy: st}, &tls.Config{ServerName: host, NextProtos: []string{"http/1.1"}, RootCAs: o.RootCAs})
	if err := c.HandshakeContext(ctx); err != nil {
		return 0, fmt.Errorf("el sıkışma: %w", err)
	}
	fmt.Fprintf(c, "HEAD / HTTP/1.1\r\nHost: %s\r\nUser-Agent: Slipstream/1.0\r\nConnection: close\r\n\r\n", host)
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("yanıt: %w", err)
	}
	if !strings.HasPrefix(line, "HTTP/") {
		return 0, fmt.Errorf("beklenmeyen yanıt: %q", strings.TrimSpace(line))
	}
	return time.Since(start), nil
}

// Conn, ilk Write çağrısını (ClientHello) stratejiye göre bölen bir sarmalayıcıdır.
type Conn struct {
	net.Conn
	Strategy desync.Strategy
	sent     bool
}

func (c *Conn) Write(b []byte) (int, error) {
	if c.sent {
		return c.Conn.Write(b)
	}
	c.sent = true
	p := c.Strategy.PlanTLS(b)
	if err := desync.Send(c.Conn, p, c.Strategy.Delay()); err != nil {
		return 0, err
	}
	return len(b), nil
}
