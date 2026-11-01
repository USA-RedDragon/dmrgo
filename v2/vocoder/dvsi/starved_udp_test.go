//go:build !tinygo

package dvsi_test

import (
	"net"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

func TestCodecStarvedClientUDP(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(3)
	srv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	addr, ok := srv.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal(srv.LocalAddr())
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	d := dvsisim.New(pairs, dvsisim.Options{Datagram: true})
	sent := make(chan struct{}, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var in, out [dvsi.MaxPacketLen]byte
		for {
			n, from, err := srv.ReadFromUDP(in[:])
			if err != nil {
				return
			}
			_, _ = d.Write(in[:n])
			replied := false
			for {
				k, _ := d.Read(out[:])
				if k == 0 {
					break
				}
				if _, err := srv.WriteToUDP(out[:k], from); err != nil {
					return
				}
				replied = true
			}
			if replied {
				sent <- struct{}{}
			}
		}
	}()
	c := starvedNew(t, &starvedPort{Port: conn, sent: sent}, dvsi.Config{Timeout: 20 * time.Millisecond, NoReset: true})
	roundTrip(t, c, pairs)
	srv.Close()
	<-done
	checkSim(t, d)
}
