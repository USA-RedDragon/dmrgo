package main

import (
	"bytes"
	"errors"
	"fmt"
	mrand "math/rand"
	"sync"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const nic = 1

type lockedSource struct {
	mu  sync.Mutex
	src mrand.Source
}

func (s *lockedSource) Int63() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.src.Int63()
}

func (s *lockedSource) Seed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.src.Seed(seed)
}

type seededReader struct {
	mu  sync.Mutex
	rng *mrand.Rand
}

func (r *seededReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Read(p)
}

type host struct {
	s    *stack.Stack
	ep   *channel.Endpoint
	addr tcpip.Address
}

var errSetup = errors.New("netstack setup failed")

func newHost(addr [4]byte, mtu uint32, clk tcpip.Clock, seed int64) (*host, error) {
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
		Clock:              clk,
		RandSource:         &lockedSource{src: mrand.NewSource(seed)},                //nolint:gosec // a simulation needs a seeded generator
		SecureRNG:          &seededReader{rng: mrand.New(mrand.NewSource(seed + 1))}, //nolint:gosec // a simulation needs a seeded generator
	})
	ep := channel.New(1024, mtu, "")
	if err := s.CreateNIC(nic, ep); err != nil {
		return nil, fmt.Errorf("%w: CreateNIC: %v", errSetup, err)
	}
	a := tcpip.AddrFrom4(addr)
	if err := s.AddProtocolAddress(nic, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: a.WithPrefix()}, stack.AddressProperties{}); err != nil {
		return nil, fmt.Errorf("%w: AddProtocolAddress: %v", errSetup, err)
	}
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: nic}})
	sack := tcpip.TCPSACKEnabled(true)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sack); err != nil {
		return nil, fmt.Errorf("%w: SACK: %v", errSetup, err)
	}
	cc := tcpip.CongestionControlOption("cubic")
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &cc); err != nil {
		return nil, fmt.Errorf("%w: cubic: %v", errSetup, err)
	}
	return &host{s: s, ep: ep, addr: a}, nil
}

func (h *host) close() {
	h.ep.Close()
	h.s.Close()
	h.s.Wait()
}

func (h *host) outbound() [][]byte {
	var out [][]byte
	for {
		pkt := h.ep.Read()
		if pkt == nil {
			return out
		}
		v := pkt.ToView()
		out = append(out, bytes.Clone(v.AsSlice()))
		v.Release()
		pkt.DecRef()
	}
}

func (h *host) inject(b []byte) {
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(bytes.Clone(b))})
	h.ep.InjectInbound(ipv4.ProtocolNumber, pkt)
	pkt.DecRef()
}

func (h *host) newEndpoint() (tcpip.Endpoint, error) {
	var wq waiter.Queue
	ep, err := h.s.NewEndpoint(tcp.ProtocolNumber, ipv4.ProtocolNumber, &wq)
	if err != nil {
		return nil, fmt.Errorf("%w: NewEndpoint: %v", errSetup, err)
	}
	return ep, nil
}

type tcpStats struct {
	segments, retransmits, timeouts, fastRetransmits, spurious uint64
}

func (h *host) tcpStats() tcpStats {
	t := h.s.Stats().TCP
	return tcpStats{
		segments:        t.SegmentsSent.Value(),
		retransmits:     t.Retransmits.Value(),
		timeouts:        t.Timeouts.Value(),
		fastRetransmits: t.FastRetransmit.Value(),
		spurious:        t.SpuriousRTORecovery.Value(),
	}
}

type hostState struct {
	sent, received uint64
	queued         int
}

func (h *host) state() hostState {
	t := h.s.Stats().TCP
	return hostState{sent: t.SegmentsSent.Value(), received: t.ValidSegmentsReceived.Value(), queued: h.ep.NumQueued()}
}
