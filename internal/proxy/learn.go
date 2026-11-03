package proxy

import (
	"strings"

	"github.com/UmutKDev/Slipstream/internal/desync"
)

// siteKey, öğrenilen stratejilerin hangi anahtarla saklanacağını belirler.
// "cdn.discord.com" ile "discord.com" aynı ISS kuralına takıldığı için
// son iki etiket kullanılır.
func siteKey(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	labels := strings.Split(host, ".")
	if len(labels) <= 2 {
		return host
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

// chainFor, bir site için denenecek stratejileri sırasıyla döndürür:
// önce o site için öğrenilen, sonra geçerli strateji, sonra adaylar.
func (e *Engine) chainFor(host string) []desync.Strategy {
	var out []desync.Strategy
	seen := map[string]bool{}
	add := func(s desync.Strategy) {
		if seen[s.Name] || len(out) >= maxAttempts {
			return
		}
		seen[s.Name] = true
		out = append(out, s)
	}
	e.learnMu.RLock()
	l, ok := e.learned[siteKey(host)]
	e.learnMu.RUnlock()
	if ok {
		add(l)
	}
	add(e.Strategy())
	for _, c := range e.Candidates {
		add(c)
	}
	return out
}

func (e *Engine) isLearned(host string, st desync.Strategy) bool {
	e.learnMu.RLock()
	defer e.learnMu.RUnlock()
	l, ok := e.learned[siteKey(host)]
	return ok && l.Name == st.Name
}

// learn, bir site için çalışan stratejiyi kaydeder. Geçerli strateji
// çalışıyorsa ayrıca kayıt tutulmaz.
func (e *Engine) learn(host string, st desync.Strategy) {
	key := siteKey(host)
	e.learnMu.Lock()
	if st.Name == e.Strategy().Name {
		if _, ok := e.learned[key]; !ok {
			e.learnMu.Unlock()
			return
		}
		delete(e.learned, key)
	} else {
		e.learned[key] = st
	}
	snap := e.learnedCopyLocked()
	e.learnMu.Unlock()
	if e.OnLearn != nil {
		e.OnLearn(snap)
	}
}

func (e *Engine) forget(host string) {
	key := siteKey(host)
	e.learnMu.Lock()
	_, ok := e.learned[key]
	delete(e.learned, key)
	snap := e.learnedCopyLocked()
	e.learnMu.Unlock()
	if ok && e.OnLearn != nil {
		e.OnLearn(snap)
	}
}

func (e *Engine) learnedCopyLocked() map[string]desync.Strategy {
	m := make(map[string]desync.Strategy, len(e.learned))
	for k, v := range e.learned {
		m[k] = v
	}
	return m
}

// Learned, öğrenilen site stratejilerinin kopyasını döndürür.
func (e *Engine) Learned() map[string]desync.Strategy {
	e.learnMu.RLock()
	defer e.learnMu.RUnlock()
	return e.learnedCopyLocked()
}

// SetLearned, öğrenilen stratejileri toptan değiştirir (diskten yükleme, sıfırlama).
func (e *Engine) SetLearned(m map[string]desync.Strategy) {
	e.learnMu.Lock()
	e.learned = make(map[string]desync.Strategy, len(m))
	for k, v := range m {
		e.learned[k] = v
	}
	e.learnMu.Unlock()
}
