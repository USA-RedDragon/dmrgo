package ipscsession

// AddrOf returns the Addr of the IP address ip and the UDP port port, as
// net.UDPAddr holds them: ip is 4 octets, or 16 octets of an IPv4-mapped
// IPv6 address. ok is false for another address and for a port outside 0
// to 65535. It does not allocate.
func AddrOf(ip []byte, port int) (a Addr, ok bool) {
	if port < 0 || port > 0xFFFF {
		return Addr{}, false
	}
	switch len(ip) {
	case 4:
	case 16:
		for i := range 10 {
			if ip[i] != 0 {
				return Addr{}, false
			}
		}
		if ip[10] != 0xFF || ip[11] != 0xFF {
			return Addr{}, false
		}
		ip = ip[12:]
	default:
		return Addr{}, false
	}
	return Addr{IP: [4]byte{ip[0], ip[1], ip[2], ip[3]}, Port: uint16(port)}, true
}
