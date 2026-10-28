package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

const (
	formPcap   = "pcap"
	formPcapng = "pcapng"
)

type datagram struct {
	data    []byte
	meta    string
	srcPort int
	dstPort int
	hint    string
}

func readInput(in []byte, format string) ([]datagram, string, []string) {
	switch format {
	case "", "auto":
		switch {
		case isPcap(in):
			format = formPcap
		case isPcapng(in):
			format = formPcapng
		case looksHex(in):
			format = "hex"
		default:
			format = "binary"
		}
	case formPcap:
		if isPcapng(in) {
			format = formPcapng
		}
	}
	switch format {
	case formPcap:
		d, errs := readPcap(in)
		return d, formPcap, errs
	case formPcapng:
		d, errs := readPcapng(in)
		return d, formPcapng, errs
	case "hex":
		d, errs := readHex(string(in))
		return d, "hex text", errs
	}
	return readBinary(in), "binary dump", nil
}

func looksHex(in []byte) bool {
	if !utf8.Valid(in) || len(in) == 0 {
		return false
	}
	h := 0
	for _, c := range in {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			h++
		case c == '\n' || c == '\r' || c == ' ' || c == '\t' || (c >= 0x20 && c < 0x7F):
		default:
			return false
		}
	}
	return h*2 >= len(in)/2
}

func isHexToken(s string) bool {
	if len(s) < 2 || len(s)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func readHex(text string) ([]datagram, []string) {
	var out []datagram
	var errs []string
	for ln, line := range strings.Split(text, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		toks := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == ',' })
		if len(toks) == 0 {
			continue
		}
		var hexToks, other []string
		for _, t := range toks {
			t2 := strings.TrimPrefix(strings.TrimPrefix(t, "0x"), "0X")
			if isHexToken(t2) {
				hexToks = append(hexToks, t2)
			} else {
				other = append(other, t)
			}
		}
		if len(hexToks) == 0 {
			if strings.TrimSpace(line) != "" {
				errs = append(errs, fmt.Sprintf("line %d: no hex octets", ln+1))
			}
			continue
		}
		var data string
		pairs := 0
		for _, t := range hexToks {
			if len(t) == 2 {
				pairs++
			}
		}
		switch {
		case len(hexToks) >= 2 && pairs == len(hexToks):
			data = strings.Join(hexToks, "")
		case len(hexToks) >= 3 && pairs == len(hexToks)-1 && len(hexToks[0]) >= 4:
			data = strings.Join(hexToks[1:], "")
			other = append(other, hexToks[0])
		default:
			best := 0
			for i, t := range hexToks {
				if len(t) > len(hexToks[best]) {
					best = i
				}
			}
			data = hexToks[best]
			for i, t := range hexToks {
				if i != best {
					other = append(other, t)
				}
			}
		}
		b, _ := hex.DecodeString(data)
		out = append(out, datagram{data: b, meta: fmt.Sprintf("line %d %s", ln+1, strings.Join(other, " "))})
	}
	return out, errs
}

func readBinary(in []byte) []datagram {
	if len(in) > 0 && len(in)%33 == 0 && in[0] != mmdvm.FrameStart {
		var out []datagram
		for off := 0; off < len(in); off += 33 {
			out = append(out, datagram{data: in[off : off+33], meta: fmt.Sprintf("octet %d", off), hint: protoBurst})
		}
		return out
	}
	if len(in) > 0 && in[0] == mmdvm.FrameStart {
		if out := splitMMDVM(in, "octet"); len(out) > 1 {
			return out
		}
	}
	return []datagram{{data: in, meta: "whole file"}}
}

func splitMMDVM(in []byte, label string) []datagram {
	var out []datagram
	off := 0
	for off < len(in) {
		_, n, st := mmdvm.Next(in[off:])
		switch st {
		case mmdvm.NextFrame:
			out = append(out, datagram{data: in[off : off+n], meta: fmt.Sprintf("%s %d", label, off), hint: protoMMDVM})
		case mmdvm.NextSkip:
		default:
			out = append(out, datagram{data: in[off:], meta: fmt.Sprintf("%s %d, incomplete frame", label, off), hint: protoMMDVM})
			return out
		}
		if n <= 0 {
			n = 1
		}
		off += n
	}
	return out
}

func isPcap(in []byte) bool {
	if len(in) < 24 {
		return false
	}
	m := binary.LittleEndian.Uint32(in)
	return m == 0xa1b2c3d4 || m == 0xd4c3b2a1 || m == 0xa1b23c4d || m == 0x4d3cb2a1
}

func isPcapng(in []byte) bool {
	return len(in) >= 28 && binary.LittleEndian.Uint32(in) == 0x0a0d0d0a
}

func readPcap(in []byte) ([]datagram, []string) {
	var bo binary.ByteOrder = binary.LittleEndian
	m := bo.Uint32(in)
	if m == 0xd4c3b2a1 || m == 0x4d3cb2a1 {
		bo = binary.BigEndian
	}
	nano := m == 0xa1b23c4d || m == 0x4d3cb2a1
	link := int(bo.Uint32(in[20:]) & 0xFFFF)
	var out []datagram
	var errs []string
	off := 24
	for n := 1; off+16 <= len(in); n++ {
		sec, frac := bo.Uint32(in[off:]), bo.Uint32(in[off+4:])
		incl := int(bo.Uint32(in[off+8:]))
		off += 16
		if incl < 0 || off+incl > len(in) {
			errs = append(errs, fmt.Sprintf("record %d: truncated", n))
			break
		}
		ts := fmt.Sprintf("%d.%06d", sec, frac)
		if nano {
			ts = fmt.Sprintf("%d.%09d", sec, frac)
		}
		d, err := linkPayload(link, in[off:off+incl])
		off += incl
		if err != "" {
			errs = append(errs, fmt.Sprintf("record %d: %s", n, err))
			continue
		}
		d.meta = fmt.Sprintf("#%d t=%s %s", n, ts, d.meta)
		out = append(out, d)
	}
	if off < len(in) && off+16 > len(in) {
		errs = append(errs, fmt.Sprintf("%d octets after the last record", len(in)-off))
	}
	return out, errs
}

func readPcapng(in []byte) ([]datagram, []string) {
	var bo binary.ByteOrder = binary.LittleEndian
	var links []int
	var out []datagram
	var errs []string
	off, n := 0, 0
	for off+12 <= len(in) {
		typ := binary.LittleEndian.Uint32(in[off:])
		if typ == 0x0a0d0d0a {
			if binary.LittleEndian.Uint32(in[off+8:]) == 0x1a2b3c4d {
				bo = binary.LittleEndian
			} else {
				bo = binary.BigEndian
			}
			links = nil
		} else {
			typ = bo.Uint32(in[off:])
		}
		blen := int(bo.Uint32(in[off+4:]))
		if blen < 12 || off+blen > len(in) {
			errs = append(errs, fmt.Sprintf("block at octet %d: bad length %d", off, blen))
			break
		}
		body := in[off+8 : off+blen-4]
		switch typ {
		case 1:
			if len(body) >= 2 {
				links = append(links, int(bo.Uint16(body)))
			}
		case 6, 3, 2:
			n++
			var ifc, capLen int
			var pkt []byte
			var ts string
			switch typ {
			case 6:
				if len(body) < 20 {
					continue
				}
				ifc = int(bo.Uint32(body))
				hi, lo := bo.Uint32(body[4:]), bo.Uint32(body[8:])
				ts = fmt.Sprintf("%d", uint64(hi)<<32|uint64(lo))
				capLen = int(bo.Uint32(body[12:]))
				pkt = body[20:]
			case 3:
				if len(body) < 4 {
					continue
				}
				capLen = len(body) - 4
				pkt = body[4:]
			case 2:
				if len(body) < 20 {
					continue
				}
				ifc = int(bo.Uint16(body))
				capLen = int(bo.Uint32(body[12:]))
				pkt = body[20:]
			}
			if capLen < len(pkt) {
				pkt = pkt[:capLen]
			}
			if ifc >= len(links) {
				errs = append(errs, fmt.Sprintf("packet %d: no interface %d", n, ifc))
				break
			}
			d, err := linkPayload(links[ifc], pkt)
			if err != "" {
				errs = append(errs, fmt.Sprintf("packet %d: %s", n, err))
				break
			}
			d.meta = fmt.Sprintf("#%d t=%s %s", n, ts, d.meta)
			out = append(out, d)
		}
		off += blen
	}
	return out, errs
}

func linkPayload(link int, p []byte) (datagram, string) {
	switch link {
	case 1:
		if len(p) < 14 {
			return datagram{}, "short Ethernet frame"
		}
		et := binary.BigEndian.Uint16(p[12:])
		p = p[14:]
		for (et == 0x8100 || et == 0x88a8) && len(p) >= 4 {
			et = binary.BigEndian.Uint16(p[2:])
			p = p[4:]
		}
		return etherPayload(et, p)
	case 0, 108:
		if len(p) < 4 {
			return datagram{}, "short loopback frame"
		}
		return ipPayload(p[4:])
	case 101, 228, 229, 12, 14:
		return ipPayload(p)
	case 113:
		if len(p) < 16 {
			return datagram{}, "short SLL frame"
		}
		return etherPayload(binary.BigEndian.Uint16(p[14:]), p[16:])
	case 276:
		if len(p) < 20 {
			return datagram{}, "short SLL2 frame"
		}
		return etherPayload(binary.BigEndian.Uint16(p), p[20:])
	case 147:
		return datagram{data: p, meta: "user link type 147"}, ""
	}
	return datagram{}, fmt.Sprintf("link type %d is not read", link)
}

func etherPayload(et uint16, p []byte) (datagram, string) {
	if et != 0x0800 && et != 0x86DD {
		return datagram{}, fmt.Sprintf("EtherType 0x%04X is not IP", et)
	}
	return ipPayload(p)
}

func ipPayload(p []byte) (datagram, string) {
	if len(p) < 1 {
		return datagram{}, "empty IP packet"
	}
	var src, dst netip.Addr
	var proto int
	var l4 []byte
	switch p[0] >> 4 {
	case 4:
		ihl := int(p[0]&0x0F) * 4
		if len(p) < 20 || ihl < 20 || len(p) < ihl {
			return datagram{}, "short IPv4 header"
		}
		total := int(binary.BigEndian.Uint16(p[2:]))
		if total >= ihl && total <= len(p) {
			p = p[:total]
		}
		if binary.BigEndian.Uint16(p[6:])&0x3FFF != 0 {
			return datagram{}, "IPv4 fragment"
		}
		src = netip.AddrFrom4([4]byte(p[12:16]))
		dst = netip.AddrFrom4([4]byte(p[16:20]))
		proto = int(p[9])
		l4 = p[ihl:]
	case 6:
		if len(p) < 40 {
			return datagram{}, "short IPv6 header"
		}
		src = netip.AddrFrom16([16]byte(p[8:24]))
		dst = netip.AddrFrom16([16]byte(p[24:40]))
		proto = int(p[6])
		l4 = p[40:]
		for proto == 0 || proto == 43 || proto == 60 {
			if len(l4) < 8 {
				return datagram{}, "short IPv6 extension header"
			}
			proto = int(l4[0])
			l4 = l4[(int(l4[1])+1)*8:]
		}
	default:
		return datagram{}, "not an IP packet"
	}
	if proto != 17 {
		return datagram{}, fmt.Sprintf("IP protocol %d is not UDP", proto)
	}
	if len(l4) < 8 {
		return datagram{}, "short UDP header"
	}
	sp, dp := binary.BigEndian.Uint16(l4), binary.BigEndian.Uint16(l4[2:])
	ul := int(binary.BigEndian.Uint16(l4[4:]))
	body := l4[8:]
	if ul >= 8 && ul-8 <= len(body) {
		body = body[:ul-8]
	}
	return datagram{data: body, srcPort: int(sp), dstPort: int(dp),
		meta: fmt.Sprintf("%s > %s", netip.AddrPortFrom(src, sp), netip.AddrPortFrom(dst, dp))}, ""
}
