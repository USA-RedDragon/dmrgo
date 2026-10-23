package main

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	unitTerm = 3120001
	unitHost = 3120002
	port     = 23
	server   = 1
	client   = 0

	nameNegotiate = "negotiate"
	nameSetup     = "setup"
	nameSubmit    = "submit"
	nameBack      = "back"
)

type cell struct {
	duplex bool
	rate   enums.DataType
	ber    float64
	window int
	mtu    uint32
	cycles int
	seed   uint64
}

type step struct {
	from  int
	data  []byte
	name  string
	start time.Duration
	end   time.Duration
	sent  int
	got   int
}

type cellResult struct {
	cell
	setup       time.Duration
	submit      []time.Duration
	back        []time.Duration
	tcp         [2]tcpStats
	links       [2]linkStats
	air         airStats
	done        bool
	elapsed     time.Duration
	payload     int
	transferred int
}

var errStuck = errors.New("session stopped")

func script(c cell) []*step {
	rng := newRand(c.seed + 9)
	neg := negotiation()
	s := make([]*step, 0, 7+4*c.cycles)
	for i, b := range neg {
		s = append(s, &step{from: (i + 1) % 2, data: b, name: nameNegotiate})
	}
	s = append(s, &step{from: server, data: formScreen(), name: nameSetup})
	for range c.cycles {
		s = append(s,
			&step{from: client, data: formInput(rng), name: nameSubmit},
			&step{from: server, data: listScreen(rng), name: nameSubmit},
			&step{from: client, data: pf3(), name: nameBack},
			&step{from: server, data: formScreen(), name: nameBack},
		)
	}
	return s
}

type session struct {
	steps  []*step
	cur    int
	eps    [2]tcpip.Endpoint
	listen tcpip.Endpoint
	buf    bytes.Buffer
}

func (s *session) poll(now time.Duration) (bool, error) {
	moved := false
	if s.eps[server] == nil {
		ep, _, err := s.listen.Accept(nil)
		if err != nil {
			if _, ok := err.(*tcpip.ErrWouldBlock); ok {
				return false, nil
			}
			return false, fmt.Errorf("accept: %v", err)
		}
		s.eps[server] = ep
		moved = true
	}
	for s.cur < len(s.steps) {
		st := s.steps[s.cur]
		if st.sent < len(st.data) {
			if st.sent == 0 {
				st.start = now
			}
			n, err := s.eps[st.from].Write(bytes.NewReader(st.data[st.sent:]), tcpip.WriteOptions{})
			if err != nil {
				if _, ok := err.(*tcpip.ErrWouldBlock); !ok {
					return moved, fmt.Errorf("write: %v", err)
				}
			}
			if n > 0 {
				st.sent += int(n)
				moved = true
			}
		}
		s.buf.Reset()
		res, err := s.eps[1-st.from].Read(&s.buf, tcpip.ReadOptions{})
		if err != nil {
			if _, ok := err.(*tcpip.ErrWouldBlock); ok {
				return moved, nil
			}
			return moved, fmt.Errorf("read: %v", err)
		}
		if res.Count == 0 {
			return moved, nil
		}
		moved = true
		st.got += res.Count
		if st.got > len(st.data) {
			return moved, fmt.Errorf("step %d got %d of %d octets", s.cur, st.got, len(st.data))
		}
		if st.got == len(st.data) {
			st.end = now
			s.cur++
		}
	}
	return moved, nil
}

type epState struct {
	ready  waiter.EventMask
	queued int
}

type worldState struct {
	hosts [2]hostState
	eps   [3]epState
}

func (s *session) state() [3]epState {
	var st [3]epState
	for i, ep := range [3]tcpip.Endpoint{s.listen, s.eps[0], s.eps[1]} {
		if ep == nil {
			continue
		}
		st[i].ready = ep.Readiness(waiter.ReadableEvents | waiter.WritableEvents | waiter.EventHUp | waiter.EventErr)
		if q, err := ep.GetSockOptInt(tcpip.ReceiveQueueSizeOption); err == nil {
			st[i].queued = q
		}
	}
	return st
}

type world struct {
	c     cell
	clk   *simClock
	airs  []*air
	hosts [2]*host
	links [2]*link
	s     *session
	dirty bool
	now   time.Duration
}

func (w *world) state() worldState {
	return worldState{hosts: [2]hostState{w.hosts[0].state(), w.hosts[1].state()}, eps: w.s.state()}
}

func (w *world) settle() {
	last, stable := w.state(), 0
	for stable < 4 {
		runtime.Gosched()
		time.Sleep(150 * time.Microsecond)
		if v := w.state(); v == last {
			stable++
		} else {
			last, stable = v, 0
		}
	}
}

func newWorld(c cell) (*world, error) {
	w := &world{c: c, clk: &simClock{}, dirty: true}
	w.airs = []*air{{rng: newRand(c.seed), ber: c.ber, senders: [2]bool{true, true}}}
	if c.duplex {
		w.airs = []*air{
			{rng: newRand(c.seed), ber: c.ber, senders: [2]bool{true, false}},
			{rng: newRand(c.seed + 100), ber: c.ber, senders: [2]bool{false, true}},
		}
	}
	ids := [2]uint32{unitTerm, unitHost}
	for k := range 2 {
		h, err := newHost(ip.RadioAddress(ip.DefaultNetwork, ids[k]), c.mtu, w.clk, int64(c.seed)+int64(10*k)) //nolint:gosec // small seeds
		if err != nil {
			w.close()
			return nil, err
		}
		w.hosts[k] = h
		own, peer := w.airs[0], w.airs[0]
		if c.duplex {
			own, peer = w.airs[k], w.airs[1-k]
		}
		l, err := newLink(ids[k], ids[1-k], c.rate, c.window, c.seed+uint64(k)+1, own.emit(k), peer.emit(k))
		if err != nil {
			w.close()
			return nil, err
		}
		l.inbound = func(_ time.Duration, pkt []byte) {
			h.inject(pkt)
			w.dirty = true
		}
		w.links[k] = l
	}
	for _, a := range w.airs {
		a.links = w.links
	}
	w.s = &session{steps: script(c)}
	if err := w.open(); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

func (w *world) open() error {
	var err error
	s := w.s
	if s.listen, err = w.hosts[server].newEndpoint(); err != nil {
		return err
	}
	if e := s.listen.Bind(tcpip.FullAddress{Port: port}); e != nil {
		return fmt.Errorf("bind: %v", e)
	}
	if e := s.listen.Listen(16); e != nil {
		return fmt.Errorf("listen: %v", e)
	}
	if s.eps[client], err = w.hosts[client].newEndpoint(); err != nil {
		return err
	}
	if e := s.eps[client].Connect(tcpip.FullAddress{Addr: w.hosts[server].addr, Port: port}); e != nil {
		if _, ok := e.(*tcpip.ErrConnectStarted); !ok {
			return fmt.Errorf("connect: %v", e)
		}
	}
	return nil
}

func (w *world) close() {
	if w.s != nil {
		for _, ep := range [3]tcpip.Endpoint{w.s.listen, w.s.eps[0], w.s.eps[1]} {
			if ep != nil {
				ep.Close()
			}
		}
	}
	for _, h := range w.hosts {
		if h != nil {
			h.close()
		}
	}
}

func (w *world) next() (time.Duration, bool) {
	next, ok := time.Duration(0), false
	ds := make([]func() (time.Duration, bool), 0, 3+len(w.airs))
	ds = append(ds, w.links[0].deadline, w.links[1].deadline, w.clk.next)
	for _, a := range w.airs {
		ds = append(ds, a.deadline)
	}
	for _, d := range ds {
		if at, dok := d(); dok && (!ok || at < next) {
			next, ok = at, true
		}
	}
	return next, ok
}

func (w *world) stopped() error {
	if err := w.s.eps[client].LastError(); err != nil {
		return fmt.Errorf("%w at %v: terminal TCP error: %v", errStuck, w.now, err)
	}
	if w.s.eps[server] == nil {
		return fmt.Errorf("%w at %v: half open, the host dropped the handshake and the terminal waits", errStuck, w.now)
	}
	return fmt.Errorf("%w at %v, record %d", errStuck, w.now, w.s.cur)
}

func (w *world) run() error {
	s := w.s
	limit := time.Duration(w.c.cycles+2) * 10 * time.Minute
	for s.cur < len(s.steps) {
		if w.dirty {
			w.settle()
			w.dirty = false
		}
		moved, err := s.poll(w.now)
		if err != nil {
			return fmt.Errorf("%v at %v: %w", w.c.rate, w.now, err)
		}
		for k := range 2 {
			for _, p := range w.hosts[k].outbound() {
				w.links[k].send(w.now, p)
			}
		}
		if moved {
			w.dirty = true
			continue
		}
		if s.cur == len(s.steps) {
			break
		}
		next, ok := w.next()
		if !ok {
			return w.stopped()
		}
		w.now = max(w.now, next)
		if w.now > limit {
			return fmt.Errorf("%w: not done after %v, at record %d of %d", errStuck, limit, s.cur, len(s.steps))
		}
		if w.clk.advance(w.now) > 0 {
			w.dirty = true
		}
		for _, a := range w.airs {
			a.tick(w.now)
		}
		w.links[0].tick(w.now)
		w.links[1].tick(w.now)
	}
	return nil
}

func runCell(c cell) (cellResult, error) {
	res := cellResult{cell: c}
	w, err := newWorld(c)
	if err != nil {
		return res, err
	}
	defer w.close()
	for _, st := range w.s.steps {
		res.payload += len(st.data)
	}
	if err := w.run(); err != nil {
		return res, err
	}
	res.done, res.elapsed = true, w.now
	steps := w.s.steps
	for i, st := range steps {
		res.transferred += st.got
		switch {
		case st.name == nameSetup:
			res.setup = st.end
		case st.from == server && st.name == nameSubmit:
			res.submit = append(res.submit, st.end-steps[i-1].start)
		case st.from == server && st.name == nameBack:
			res.back = append(res.back, st.end-steps[i-1].start)
		}
	}
	for k := range 2 {
		res.tcp[k], res.links[k] = w.hosts[k].tcpStats(), w.links[k].stats
	}
	for _, a := range w.airs {
		res.air.bursts += a.stats.bursts
		res.air.collisions += a.stats.collisions
	}
	return res, nil
}

func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	return s[min(len(s)-1, int(p*float64(len(s))))]
}

func mean(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	var sum time.Duration
	for _, x := range d {
		sum += x
	}
	return sum / time.Duration(len(d))
}
