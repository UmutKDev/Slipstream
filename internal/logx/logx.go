// Package logx, arayüzde gösterilen canlı günlük ve sayaçları tutar.
package logx

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Entry, tek bir günlük satırıdır.
type Entry struct {
	Seq   uint64    `json:"seq"`
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

// Ring, son N günlük satırını tutan halka arabelleğidir.
type Ring struct {
	mu   sync.Mutex
	buf  []Entry
	next int
	seq  uint64
	full bool
}

// NewRing, n kapasiteli bir arabellek kurar.
func NewRing(n int) *Ring { return &Ring{buf: make([]Entry, n)} }

func (r *Ring) add(level, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	log.Printf("[%s] %s", level, msg)
	r.mu.Lock()
	r.seq++
	r.buf[r.next] = Entry{Seq: r.seq, Time: time.Now(), Level: level, Msg: msg}
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.full = true
	}
	r.mu.Unlock()
}

func (r *Ring) Infof(format string, a ...any)  { r.add("info", format, a...) }
func (r *Ring) Warnf(format string, a ...any)  { r.add("warn", format, a...) }
func (r *Ring) Errorf(format string, a ...any) { r.add("error", format, a...) }

// Since, sıra numarası since'ten büyük olan satırları eskiden yeniye döndürür.
func (r *Ring) Since(since uint64) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Entry
	n := len(r.buf)
	start := 0
	if r.full {
		start = r.next
	}
	for i := 0; i < n; i++ {
		e := r.buf[(start+i)%n]
		if e.Seq > since {
			out = append(out, e)
		}
	}
	return out
}

// Stats, bağlantı ve trafik sayaçlarıdır.
type Stats struct {
	Total    atomic.Int64
	Active   atomic.Int64
	Bypassed atomic.Int64
	Failed   atomic.Int64
	BytesUp  atomic.Int64
	BytesDn  atomic.Int64
	started  time.Time
}

// NewStats, sayaçları sıfırdan başlatır.
func NewStats() *Stats { return &Stats{started: time.Now()} }

// Snapshot, sayaçların JSON'a uygun anlık görüntüsüdür.
type Snapshot struct {
	Total     int64 `json:"total"`
	Active    int64 `json:"active"`
	Bypassed  int64 `json:"bypassed"`
	Failed    int64 `json:"failed"`
	BytesUp   int64 `json:"bytesUp"`
	BytesDown int64 `json:"bytesDown"`
	UptimeSec int64 `json:"uptimeSec"`
}

// Snapshot, anlık görüntü alır.
func (s *Stats) Snapshot() Snapshot {
	return Snapshot{
		Total: s.Total.Load(), Active: s.Active.Load(), Bypassed: s.Bypassed.Load(),
		Failed: s.Failed.Load(), BytesUp: s.BytesUp.Load(), BytesDown: s.BytesDn.Load(),
		UptimeSec: int64(time.Since(s.started).Seconds()),
	}
}
