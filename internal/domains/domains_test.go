package domains

import (
	"strings"
	"testing"
)

func TestMatcher(t *testing.T) {
	m := NewMatcher(ModeList, []string{"discord.com", "*.Roblox.com."})
	cases := map[string]bool{
		"discord.com": true, "cdn.discord.com": true, "DISCORD.COM": true,
		"notdiscord.com": false, "www.roblox.com": true, "google.com": false,
	}
	for h, want := range cases {
		if got := m.Apply(h); got != want {
			t.Errorf("%s: %v, beklenen %v", h, got, want)
		}
	}
	m.SetMode(ModeAll)
	if !m.Apply("google.com") {
		t.Error("all kipinde her şey eşleşmeli")
	}
}

func TestParse(t *testing.T) {
	got := Parse(strings.NewReader("# yorum\n discord.com \n\nimgur.com\n"))
	if len(got) != 2 || got[0] != "discord.com" {
		t.Fatalf("%v", got)
	}
}
