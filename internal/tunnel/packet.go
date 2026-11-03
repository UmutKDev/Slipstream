// Package tunnel, iOS Packet Tunnel eklentisinin Go tarafıdır. Eklenti
// yalnız sahte DNS sunucusunun adresini tünele yönlendirir. Buradaki kod o
// adrese gelen UDP paketlerini çözer, DNS sorgusunu yanıtlar ve yanıt
// paketini kurar. Web trafiği tünelden değil, tünelin proxy ayarı üzerinden
// yerel motora gelir.
package tunnel

import (
	"encoding/binary"
	"errors"
)

// UDPPacket, IPv4 üzerindeki bir UDP datagramıdır.
type UDPPacket struct {
	Src, Dst         [4]byte
	SrcPort, DstPort uint16
	Payload          []byte
}

var errNotUDP4 = errors.New("tunnel: IPv4 UDP paketi değil")

// ParseUDP4, ham IP paketini çözer. Parçalanmış ya da bozuk paketleri reddeder.
func ParseUDP4(pkt []byte) (UDPPacket, error) {
	var p UDPPacket
	if len(pkt) < 28 || pkt[0]>>4 != 4 {
		return p, errNotUDP4
	}
	ihl := int(pkt[0]&0x0f) * 4
	total := int(binary.BigEndian.Uint16(pkt[2:4]))
	if ihl < 20 || total > len(pkt) || total < ihl+8 || pkt[9] != 17 {
		return p, errNotUDP4
	}
	// Parça bayrağı (MF) ya da parça kayması varsa yok say.
	if binary.BigEndian.Uint16(pkt[6:8])&0x3fff != 0 {
		return p, errNotUDP4
	}
	copy(p.Src[:], pkt[12:16])
	copy(p.Dst[:], pkt[16:20])
	u := pkt[ihl:total]
	p.SrcPort = binary.BigEndian.Uint16(u[0:2])
	p.DstPort = binary.BigEndian.Uint16(u[2:4])
	ulen := int(binary.BigEndian.Uint16(u[4:6]))
	if ulen < 8 || ulen > len(u) {
		return p, errNotUDP4
	}
	p.Payload = u[8:ulen]
	return p, nil
}

// Reply, gelen paketin adreslerini ve portlarını ters çevirip payload ile
// yeni bir IPv4 UDP paketi kurar.
func (p UDPPacket) Reply(payload []byte) []byte {
	return BuildUDP4(p.Dst, p.Src, p.DstPort, p.SrcPort, payload)
}

// BuildUDP4, sağlama toplamları hesaplanmış bir IPv4 UDP paketi kurar.
func BuildUDP4(src, dst [4]byte, sport, dport uint16, payload []byte) []byte {
	total := 20 + 8 + len(payload)
	b := make([]byte, total)
	b[0] = 0x45
	binary.BigEndian.PutUint16(b[2:4], uint16(total))
	binary.BigEndian.PutUint16(b[6:8], 0x4000) // parçalama yok
	b[8] = 64
	b[9] = 17
	copy(b[12:16], src[:])
	copy(b[16:20], dst[:])
	binary.BigEndian.PutUint16(b[10:12], checksum(b[:20], 0))

	u := b[20:]
	binary.BigEndian.PutUint16(u[0:2], sport)
	binary.BigEndian.PutUint16(u[2:4], dport)
	binary.BigEndian.PutUint16(u[4:6], uint16(8+len(payload)))
	copy(u[8:], payload)
	// Sözde başlık: kaynak, hedef, protokol, UDP uzunluğu.
	var pseudo uint32
	pseudo += uint32(binary.BigEndian.Uint16(src[0:2])) + uint32(binary.BigEndian.Uint16(src[2:4]))
	pseudo += uint32(binary.BigEndian.Uint16(dst[0:2])) + uint32(binary.BigEndian.Uint16(dst[2:4]))
	pseudo += 17 + uint32(len(u))
	cs := checksum(u, pseudo)
	if cs == 0 {
		cs = 0xffff
	}
	binary.BigEndian.PutUint16(u[6:8], cs)
	return b
}

func checksum(b []byte, sum uint32) uint16 {
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
