// Package tlsx, TLS ClientHello kayıtlarını ayrıştırır ve SNI konumunu bulur.
package tlsx

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	RecordTypeHandshake  = 0x16
	HandshakeClientHello = 0x01
	ExtServerName        = 0x0000
	RecordHeaderLen      = 5
	maxRecordLen         = 1<<14 + 2048
)

var ErrNotClientHello = errors.New("tlsx: ClientHello değil")

// SNIInfo, kayıt içindeki SNI ana bilgisayar adının konumunu tutar.
// Offset, tam kayıt (başlık dahil) içindeki bayt konumudur.
type SNIInfo struct {
	Host   string
	Offset int
	Length int
}

// IsTLSHandshake, ilk baytlara bakarak TLS el sıkışması olup olmadığını söyler.
func IsTLSHandshake(b []byte) bool {
	return len(b) >= 3 && b[0] == RecordTypeHandshake && b[1] == 0x03 && b[2] <= 0x04
}

// ReadRecord, r'den tek bir TLS kaydını (başlık dahil) eksiksiz okur.
func ReadRecord(r io.Reader) ([]byte, error) {
	hdr := make([]byte, RecordHeaderLen)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	if !IsTLSHandshake(hdr) {
		return hdr, ErrNotClientHello
	}
	n := int(binary.BigEndian.Uint16(hdr[3:5]))
	if n > maxRecordLen {
		return hdr, ErrNotClientHello
	}
	buf := make([]byte, RecordHeaderLen+n)
	copy(buf, hdr)
	if _, err := io.ReadFull(r, buf[RecordHeaderLen:]); err != nil {
		return nil, err
	}
	return buf, nil
}

// ParseSNI, tam bir ClientHello kaydında SNI'yi bulur.
func ParseSNI(rec []byte) (SNIInfo, error) {
	if !IsTLSHandshake(rec) || len(rec) < RecordHeaderLen+4 {
		return SNIInfo{}, ErrNotClientHello
	}
	p := RecordHeaderLen
	if rec[p] != HandshakeClientHello {
		return SNIInfo{}, ErrNotClientHello
	}
	p += 4      // tür + 3 bayt uzunluk
	p += 2 + 32 // sürüm + random
	if p >= len(rec) {
		return SNIInfo{}, ErrNotClientHello
	}
	p += 1 + int(rec[p]) // oturum kimliği
	if p+2 > len(rec) {
		return SNIInfo{}, ErrNotClientHello
	}
	p += 2 + int(binary.BigEndian.Uint16(rec[p:])) // şifre takımları
	if p >= len(rec) {
		return SNIInfo{}, ErrNotClientHello
	}
	p += 1 + int(rec[p]) // sıkıştırma
	if p+2 > len(rec) {
		return SNIInfo{}, ErrNotClientHello
	}
	extEnd := p + 2 + int(binary.BigEndian.Uint16(rec[p:]))
	p += 2
	if extEnd > len(rec) {
		extEnd = len(rec)
	}
	for p+4 <= extEnd {
		typ := binary.BigEndian.Uint16(rec[p:])
		l := int(binary.BigEndian.Uint16(rec[p+2:]))
		body := p + 4
		if body+l > extEnd {
			break
		}
		if typ == ExtServerName && l >= 5 {
			// liste uzunluğu (2), ad türü (1), ad uzunluğu (2)
			if rec[body+2] == 0 {
				nl := int(binary.BigEndian.Uint16(rec[body+3:]))
				off := body + 5
				if off+nl <= body+l {
					return SNIInfo{Host: string(rec[off : off+nl]), Offset: off, Length: nl}, nil
				}
			}
		}
		p = body + l
	}
	return SNIInfo{}, errors.New("tlsx: SNI yok")
}

// SplitRecord, tek bir el sıkışma kaydını verilen yük konumlarından birden
// çok TLS kaydına böler. Konumlar yük (başlık hariç) içindeki baytlardır.
// TLS standardı el sıkışma mesajlarının kayıtlara bölünmesine izin verir,
// ama basit DPI cihazları yalnız ilk kayda bakar.
func SplitRecord(rec []byte, cuts ...int) [][]byte {
	if len(rec) < RecordHeaderLen {
		return [][]byte{rec}
	}
	payload := rec[RecordHeaderLen:]
	var out [][]byte
	prev := 0
	emit := func(a, b int) {
		r := make([]byte, RecordHeaderLen+(b-a))
		copy(r, rec[:3])
		binary.BigEndian.PutUint16(r[3:], uint16(b-a))
		copy(r[RecordHeaderLen:], payload[a:b])
		out = append(out, r)
	}
	for _, c := range cuts {
		if c <= prev || c >= len(payload) {
			continue
		}
		emit(prev, c)
		prev = c
	}
	emit(prev, len(payload))
	return out
}
