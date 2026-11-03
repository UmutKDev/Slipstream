package tunnel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/UmutKDev/Slipstream/internal/adblock"
	"github.com/UmutKDev/Slipstream/internal/dns"
	"golang.org/x/net/dns/dnsmessage"
)

// DNS, cihazın DNS sorgularını yanıtlar. Reklam listesindeki alan adları
// için 0.0.0.0 döner, gerisini motorun DoH çözücüsüne iletir. Böylece ISS'nin
// DNS zehirlemesi aşılır ve reklam engelleme proxy kullanmayan uygulamalarda
// da çalışır.
type DNS struct {
	// Resolver, geçerli çözücüyü verir (kullanıcı DNS'i değiştirebilir).
	Resolver func() *dns.Resolver
	Ads      *adblock.Blocker

	mu    sync.Mutex
	cache map[cacheKey]cached
}

type cacheKey struct {
	name  string
	qtype dnsmessage.Type
}

type cached struct {
	wire    []byte
	expires time.Time
}

const maxCache = 4096

var errBadQuery = errors.New("tunnel: DNS sorgusu çözülemedi")

// Answer, ham DNS sorgusuna ham yanıt döndürür.
func (d *DNS) Answer(ctx context.Context, q []byte) ([]byte, error) {
	var p dnsmessage.Parser
	h, err := p.Start(q)
	if err != nil || h.Response {
		return nil, errBadQuery
	}
	qs, err := p.AllQuestions()
	if err != nil || len(qs) != 1 {
		return nil, errBadQuery
	}
	question := qs[0]
	name := strings.ToLower(strings.TrimSuffix(question.Name.String(), "."))

	if d.Ads != nil && d.Ads.Block(name) {
		return blockedReply(h, question)
	}

	key := cacheKey{name, question.Type}
	if wire := d.lookup(key); wire != nil {
		wire[0], wire[1] = q[0], q[1]
		return wire, nil
	}
	resp, err := d.Resolver().Exchange(ctx, q)
	if err != nil {
		return nil, err
	}
	d.store(key, resp)
	return resp, nil
}

// Flush, önbelleği temizler.
func (d *DNS) Flush() {
	d.mu.Lock()
	d.cache = nil
	d.mu.Unlock()
}

func (d *DNS) lookup(k cacheKey) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ok := d.cache[k]
	if !ok {
		return nil
	}
	if time.Now().After(c.expires) {
		delete(d.cache, k)
		return nil
	}
	return append([]byte(nil), c.wire...)
}

func (d *DNS) store(k cacheKey, wire []byte) {
	ttl, ok := minTTL(wire)
	if !ok {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cache == nil || len(d.cache) >= maxCache {
		d.cache = make(map[cacheKey]cached, 256)
	}
	d.cache[k] = cached{append([]byte(nil), wire...), time.Now().Add(ttl)}
}

// minTTL, yanıttaki en küçük TTL'yi 30 saniye ile 1 saat arasına sıkıştırır.
// Başarısız yanıtlar ve kesik yanıtlar önbelleğe alınmaz.
func minTTL(wire []byte) (time.Duration, bool) {
	var p dnsmessage.Parser
	h, err := p.Start(wire)
	if err != nil || h.Truncated || (h.RCode != dnsmessage.RCodeSuccess && h.RCode != dnsmessage.RCodeNameError) {
		return 0, false
	}
	if err := p.SkipAllQuestions(); err != nil {
		return 0, false
	}
	ttl := uint32(3600)
	n := 0
	for {
		rh, err := p.AnswerHeader()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return 0, false
		}
		n++
		if rh.TTL < ttl {
			ttl = rh.TTL
		}
		if err := p.SkipAnswer(); err != nil {
			return 0, false
		}
	}
	if n == 0 {
		ttl = 60 // boş ya da NXDOMAIN yanıtı
	}
	if ttl < 30 {
		ttl = 30
	}
	return time.Duration(ttl) * time.Second, true
}

// blockedReply, engelli alan adı için A sorgusuna 0.0.0.0, AAAA sorgusuna ::
// döner. Diğer türlerde boş başarılı yanıt verilir; NXDOMAIN bazı
// uygulamaların sürekli yeniden denemesine yol açıyor.
func blockedReply(h dnsmessage.Header, q dnsmessage.Question) ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID: h.ID, Response: true, RecursionDesired: h.RecursionDesired, RecursionAvailable: true,
	})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(q); err != nil {
		return nil, err
	}
	if err := b.StartAnswers(); err != nil {
		return nil, err
	}
	rh := dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 300}
	switch q.Type {
	case dnsmessage.TypeA:
		if err := b.AResource(rh, dnsmessage.AResource{}); err != nil {
			return nil, err
		}
	case dnsmessage.TypeAAAA:
		if err := b.AAAAResource(rh, dnsmessage.AAAAResource{}); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}
