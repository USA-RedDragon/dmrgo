package ipscsession_test

import (
	"fmt"
	"net"
	"net/netip"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

// ExampleAddrOf converts the address of a UDP packet to an Addr, and an
// Addr back to the address to send to.
func ExampleAddrOf() {
	from := &net.UDPAddr{IP: net.ParseIP("192.168.0.10"), Port: 50000}
	a, ok := ipscsession.AddrOf(from.IP, from.Port)
	fmt.Println(a, ok)
	to := &net.UDPAddr{IP: a.IP[:], Port: int(a.Port)}
	fmt.Println(to)
	fmt.Println(netip.AddrPortFrom(netip.AddrFrom4(a.IP), a.Port))
	// Output:
	// {[192 168 0 10] 50000} true
	// 192.168.0.10:50000
	// 192.168.0.10:50000
}

func TestAddrOf(t *testing.T) {
	t.Parallel()

	v6 := net.ParseIP("2001:db8::1")
	tests := []struct {
		ip   []byte
		port int
		ok   bool
	}{
		{[]byte{10, 0, 0, 1}, 0, true},
		{net.ParseIP("10.0.0.1"), 65535, true},
		{[]byte{10, 0, 0, 1}, 65536, false},
		{[]byte{10, 0, 0, 1}, -1, false},
		{v6, 1, false},
		{nil, 1, false},
		{[]byte{10, 0, 0}, 1, false},
	}
	for _, tt := range tests {
		a, ok := ipscsession.AddrOf(tt.ip, tt.port)
		if ok != tt.ok || (ok && (a.IP != [4]byte{10, 0, 0, 1} || int(a.Port) != tt.port)) || (!ok && a != ipscsession.Addr{}) {
			t.Errorf("AddrOf(%v, %d) = %+v, %v", tt.ip, tt.port, a, ok)
		}
	}
}
