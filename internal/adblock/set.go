package adblock

import (
	"bytes"
	"sort"
	"strings"
)

// domainSet, sıralı alan adlarını tek bir dizgide tutan salt okunur bir
// kümedir. 250 bin alan adı map ile yaklaşık 20 MB tutarken burada 6-7 MB
// tutar. iOS tünel eklentisi 50 MB bellek sınırıyla çalıştığı için önemli.
type domainSet struct {
	blob string
	offs []uint32 // i. alan adı blob[offs[i]:offs[i+1]]
}

// setBuilder, alan adlarını tek tek dizgi ayırmadan bir bayt dizisinde
// toplar; yükleme sırasındaki bellek tepesini düşük tutar.
type setBuilder struct {
	blob []byte
	offs []uint32
}

func (b *setBuilder) add(d string) {
	b.offs = append(b.offs, uint32(len(b.blob)))
	b.blob = append(b.blob, d...)
}

// build, toplananları sıralar, tekrarları atar ve paketler.
func (b *setBuilder) build() *domainSet {
	n := len(b.offs)
	ends := append(b.offs, uint32(len(b.blob)))
	at := func(i uint32) []byte { return b.blob[ends[i]:ends[i+1]] }
	idx := make([]uint32, n)
	for i := range idx {
		idx[i] = uint32(i)
	}
	sort.Slice(idx, func(x, y int) bool { return bytes.Compare(at(idx[x]), at(idx[y])) < 0 })

	var sb strings.Builder
	sb.Grow(len(b.blob))
	offs := make([]uint32, 0, n+1)
	var prev []byte
	for k, i := range idx {
		d := at(i)
		if k > 0 && bytes.Equal(d, prev) {
			continue
		}
		offs = append(offs, uint32(sb.Len()))
		sb.Write(d)
		prev = d
	}
	offs = append(offs, uint32(sb.Len()))
	b.blob, b.offs = nil, nil
	// Tekrarlar atıldıysa fazla kapasiteyi bırak.
	if cap(offs)-len(offs) > len(offs)/4 {
		offs = append([]uint32(nil), offs...)
	}
	return &domainSet{blob: sb.String(), offs: offs}
}

// newDomainSet, verilen alan adlarından bir küme kurar.
func newDomainSet(list []string) *domainSet {
	var b setBuilder
	for _, d := range list {
		b.add(d)
	}
	return b.build()
}

func (s *domainSet) Len() int {
	if s == nil || len(s.offs) == 0 {
		return 0
	}
	return len(s.offs) - 1
}

func (s *domainSet) at(i int) string { return s.blob[s.offs[i]:s.offs[i+1]] }

// Has, alan adının kümede olup olmadığını ikili aramayla bulur.
func (s *domainSet) Has(d string) bool {
	n := s.Len()
	i := sort.Search(n, func(i int) bool { return s.at(i) >= d })
	return i < n && s.at(i) == d
}
