package adblock

import (
	"strings"
	"testing"
)

func TestParseAndBlock(t *testing.T) {
	src := `# yorum
! adblock yorumu
0.0.0.0 ads.example.com
127.0.0.1 tracker.example.net
||doubleclick.net^
||ads.cdn.com^$third-party
||example.org/path^
@@||allowed.com^
0.0.0.0 0.0.0.0
`
	set := map[string]struct{}{}
	Parse(strings.NewReader(src), set)
	for _, d := range []string{"ads.example.com", "tracker.example.net", "doubleclick.net"} {
		if _, ok := set[d]; !ok {
			t.Errorf("%s eksik", d)
		}
	}
	if len(set) != 3 {
		t.Errorf("küme %v", set)
	}
	b := New(t.TempDir())
	b.set = set
	if b.Block("ads.example.com") {
		t.Error("kapalıyken engellememeli")
	}
	b.SetEnabled(true)
	if !b.Block("stats.g.doubleclick.net") || b.Block("example.com") {
		t.Error("üst alan adı eşleşmesi yanlış")
	}
	b.SetAllow([]string{"doubleclick.net"})
	if b.Block("doubleclick.net") {
		t.Error("istisna çalışmadı")
	}
	if b.Blocked.Load() != 1 {
		t.Errorf("sayaç %d", b.Blocked.Load())
	}
}

func TestParsePlainDomains(t *testing.T) {
	set := map[string]struct{}{}
	Parse(strings.NewReader("# HaGeZi\nads.example.com\n*.pop.example.net\n0.0.0.0  turk.reklam.com\n::1 localhost\nbad line here\n||x.com^$third-party\n"), set)
	for _, d := range []string{"ads.example.com", "pop.example.net", "turk.reklam.com"} {
		if _, ok := set[d]; !ok {
			t.Errorf("%s bekleniyordu", d)
		}
	}
	if len(set) != 3 {
		t.Errorf("3 alan adı bekleniyordu, %d: %v", len(set), set)
	}
}

func TestLookupDefaults(t *testing.T) {
	ids := DefaultIDs()
	if len(ids) == 0 || len(Lookup(nil)) != len(ids) {
		t.Fatal("varsayılan listeler boş")
	}
	if got := Lookup([]string{"turk-adlist", "yok"}); len(got) != 1 || got[0].ID != "turk-adlist" {
		t.Fatalf("Lookup: %+v", got)
	}
}
