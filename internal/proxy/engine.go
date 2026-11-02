// Package proxy, Slipstream'in yerel HTTP ve SOCKS5 proxy motorudur. Tek bir
// port üzerinden iki protokolü de ilk bayta bakarak ayırt eder.
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UmutKDev/slipstream/internal/desync"
	"github.com/UmutKDev/slipstream/internal/dns"
	"github.com/UmutKDev/slipstream/internal/domains"
	"github.com/UmutKDev/slipstream/internal/logx"
)

// Engine, proxy sunucusunun durumunu tutar. Strateji ve çözücü çalışırken
// değiştirilebilir.
type Engine struct {
	strategy atomic.Pointer[desync.Strategy]
	resolver atomic.Pointer[dns.Resolver]
	Domains  *domains.Matcher
	Log      *logx.Ring
	Stats    *logx.Stats

	DialTimeout time.Duration

	mu sync.Mutex
	ln net.Listener
}

// NewEngine, bir motor kurar.
func NewEngine(s desync.Strategy, r *dns.Resolver, m *domains.Matcher) *Engine {
	e := &Engine{Domains: m, Log: logx.NewRing(500), Stats: logx.NewStats(), DialTimeout: 8 * time.Second}
	e.SetStrategy(s)
	e.SetResolver(r)
	return e
}

func (e *Engine) SetStrategy(s desync.Strategy) { e.strategy.Store(&s) }
func (e *Engine) Strategy() desync.Strategy     { return *e.strategy.Load() }
func (e *Engine) SetResolver(r *dns.Resolver)   { e.resolver.Store(r) }
func (e *Engine) Resolver() *dns.Resolver       { return e.resolver.Load() }

// ListenAndServe, addr üzerinde dinler ve ctx iptal edilene kadar çalışır.
func (e *Engine) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return e.Serve(ctx, ln)
}

// Serve, verilen dinleyiciyi kullanır.
func (e *Engine) Serve(ctx context.Context, ln net.Listener) error {
	e.mu.Lock()
	e.ln = ln
	e.mu.Unlock()
	go func() { <-ctx.Done(); ln.Close() }()
	e.Log.Infof("proxy %s adresinde dinliyor", ln.Addr())
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return err
		}
		go e.handle(c)
	}
}

func (e *Engine) handle(c net.Conn) {
	e.Stats.Total.Add(1)
	e.Stats.Active.Add(1)
	defer e.Stats.Active.Add(-1)
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReaderSize(c, 32<<10)
	first, err := br.Peek(1)
	if err != nil {
		return
	}
	if first[0] == 0x05 {
		e.serveSOCKS(c, br)
	} else {
		e.serveHTTP(c, br)
	}
}

// Dial, alan adını DoH ile çözer ve ilk yanıt veren adrese bağlanır.
func (e *Engine) Dial(ctx context.Context, host string, port int) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, e.DialTimeout)
	defer cancel()
	ips, err := e.Resolver().Lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("%s çözülemedi: %w", host, err)
	}
	d := net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}
	var last error
	for i, ip := range ips {
		if i >= 4 {
			break
		}
		c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
		if err == nil {
			return c, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("adres yok")
	}
	return nil, fmt.Errorf("%s:%d bağlanılamadı: %w", host, port, last)
}
