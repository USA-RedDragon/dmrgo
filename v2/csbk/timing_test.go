package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type ctf struct {
	gen uint8
	sa  uint16
	ldi uint8
	lid uint32
	nl  bool
	cto uint8
	sdi uint8
	sid uint32
}

func ctBurst(t *testing.T, f ctf) *layer2.RawBurst {
	t.Helper()
	p := pdu.ChannelTimingPDU{Generation: f.gen, SyncAge: f.sa, LeaderDynamicIdentifier: f.ldi, LeaderIdentifier: f.lid, NewLeader: f.nl,
		ChannelTimingOp1: f.cto&2 != 0, ChannelTimingOp0: f.cto&1 != 0, SourceDynamicIdentifier: f.sdi, SourceIdentifier: f.sid}
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKChannelTiming, ChannelTimingPDU: &p}
	b, err := layer2.BuildBurst(&c, csbk.AllSiteColorCode, layer2.SyncSourceDirectTS2)
	if err != nil {
		t.Fatal(err)
	}
	return &b
}

func ctOf(t *testing.T, raw layer2.RawBurst) ctf {
	t.Helper()
	var b layer2.Burst
	if err := b.DecodeFromBytes(raw); err != nil {
		t.Fatal(err)
	}
	c, ok := b.Data.(*pdu.CSBK)
	if !ok || c.ChannelTimingPDU == nil || b.SlotType.ColorCode != csbk.AllSiteColorCode {
		t.Fatalf("not a CT_CSBK: %T, color code %d", b.Data, b.SlotType.ColorCode)
	}
	p := c.ChannelTimingPDU
	var cto uint8
	if p.ChannelTimingOp1 {
		cto = 2
	}
	if p.ChannelTimingOp0 {
		cto |= 1
	}
	return ctf{gen: p.Generation, sa: p.SyncAge, ldi: p.LeaderDynamicIdentifier, lid: p.LeaderIdentifier, nl: p.NewLeader, cto: cto, sdi: p.SourceDynamicIdentifier, sid: p.SourceIdentifier}
}

type tx struct {
	f     ctf
	first time.Duration
	n     int
}

type tdriver struct {
	t   *testing.T
	m   *csbk.Timing
	now time.Duration
	out []csbk.Output
	all []csbk.Output
}

func newTiming(t *testing.T, id uint32, di uint8, seed uint64) *tdriver {
	t.Helper()
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: id, DynamicID: di, Slot: enums.Slot1, Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	m.Join(0)
	return &tdriver{t: t, m: m, out: make([]csbk.Output, 0, csbk.MaxOutputs)}
}

func (d *tdriver) keep(out []csbk.Output) {
	d.t.Helper()
	if len(out) > csbk.MaxOutputs {
		d.t.Fatalf("%d outputs", len(out))
	}
	d.all = append(d.all, out...)
}

func (d *tdriver) until(end time.Duration) []csbk.Output {
	d.t.Helper()
	mark := len(d.all)
	for range 1000 {
		at, ok := d.m.Deadline()
		if !ok || at > end {
			break
		}
		d.now = max(d.now, at)
		d.keep(d.m.Tick(d.now, d.out[:0]))
	}
	d.now = max(d.now, end)
	return d.all[mark:]
}

func (d *tdriver) rx(at time.Duration, f ctf) []csbk.Output {
	d.t.Helper()
	d.until(at)
	mark := len(d.all)
	d.keep(d.m.Burst(at, ctBurst(d.t, f), layer2.Hint{}, d.out[:0]))
	return d.all[mark:]
}

func (d *tdriver) txs(out []csbk.Output) []tx {
	d.t.Helper()
	var got []tx
	for _, o := range out {
		if o.Kind != csbk.OutputBurst {
			continue
		}
		if o.Service != csbk.ServiceTiming {
			d.t.Fatalf("service %v", o.Service)
		}
		f := ctOf(d.t, o.Burst)
		if n := len(got); n > 0 && got[n-1].f == f && o.Due == got[n-1].first+time.Duration(got[n-1].n)*csbk.BurstInterval {
			got[n-1].n++
			continue
		}
		got = append(got, tx{f: f, first: o.Due, n: 1})
	}
	return got
}

func timings(out []csbk.Output) int {
	n := 0
	for _, o := range out {
		if o.Kind == csbk.OutputTiming {
			n++
		}
	}
	return n
}

func (d *tdriver) state(want csbk.TimingState) {
	d.t.Helper()
	if s := d.m.State(); s != want {
		d.t.Fatalf("state %v, want %v", s, want)
	}
}

func (d *tdriver) leader(di uint8, id uint32) {
	d.t.Helper()
	gdi, gid, ok := d.m.Leader()
	if ok != (id != 0) || gdi != di || gid != id {
		d.t.Fatalf("leader %d/%d %v, want %d/%d", gdi, gid, ok, di, id)
	}
}

const (
	minute  = time.Minute
	rhotMax = constants.CTRHOTMax
)

func beaconOf(di uint8, id uint32) ctf {
	return ctf{ldi: di, lid: id, cto: 3, sdi: di, sid: id}
}

func following(t *testing.T) *tdriver {
	t.Helper()
	d := newTiming(t, 100, 2, 1)
	if n := timings(d.rx(minute, ctf{gen: 1, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 55})); n != 1 {
		t.Fatalf("%d timing outputs", n)
	}
	d.state(csbk.TimingLeaderAndTimingKnown)
	d.leader(3, 7)
	return d
}

// ETSI TS 102 361-2 clauses 6.2.3.1 and 6.2.3.2
func TestTimingNoLeaderRequest(t *testing.T) {
	t.Parallel()

	d := newTiming(t, 100, 2, 1)
	d.state(csbk.TimingLeaderAndTimingUnknown)
	if at, ok := d.m.Deadline(); !ok || at != constants.NoLeader {
		t.Fatalf("deadline %v %v", at, ok)
	}
	got := d.txs(d.until(constants.NoLeader + rhotMax + time.Second))
	want := ctf{sdi: 2, sid: 100}
	if len(got) != 1 || got[0].f != want || got[0].n != 3 || got[0].first < constants.NoLeader || got[0].first > constants.NoLeader+rhotMax {
		t.Fatalf("sent %+v", got)
	}
	if at, ok := d.m.Deadline(); !ok || at != got[0].first+constants.NoLeader {
		t.Fatalf("NoLeader not set again: %v", at)
	}
	d.state(csbk.TimingLeaderAndTimingUnknown)
	d.leader(0, 0)
}

// ETSI TS 102 361-2 clause 6.2.3.2
func TestTimingRequestWaitsForLowerLeader(t *testing.T) {
	t.Parallel()

	d := newTiming(t, 100, 2, 3)
	d.until(constants.NoLeader)
	out := d.rx(constants.NoLeader+10*time.Millisecond, ctf{gen: 1, ldi: 1, lid: 9, cto: 2, sid: 8})
	if timings(out) != 1 {
		t.Fatalf("outputs %v", kinds(out))
	}
	d.state(csbk.TimingLeaderAndTimingUnknown)
	if got := d.txs(d.until(constants.NoLeader + rhotMax + time.Second)); len(got) != 1 || got[0].f.cto != 0 {
		t.Fatalf("request not kept: %+v", got)
	}
	e := newTiming(t, 100, 2, 3)
	e.until(constants.NoLeader)
	e.rx(constants.NoLeader+10*time.Millisecond, ctf{gen: 1, ldi: 3, lid: 9, cto: 2, sid: 8})
	e.state(csbk.TimingLeaderAndTimingKnown)
	e.leader(3, 9)
	if got := e.txs(e.until(constants.NoLeader + rhotMax + time.Second)); len(got) != 0 {
		t.Fatalf("request not cancelled: %+v", got)
	}
}

// ETSI TS 102 361-2 clauses 6.2.3.7 and 6.2.3.9
func TestTimingAcceptLeader(t *testing.T) {
	t.Parallel()

	d := following(t)
	if got := d.txs(d.until(minute + rhotMax + time.Second)); len(got) != 0 {
		t.Fatalf("sent %+v", got)
	}
	out := d.m.Terminator(2*minute, nil)
	if got := d.txs(out); len(got) != 1 || got[0].f != (ctf{gen: 2, sa: 140, ldi: 3, lid: 7, cto: 2, sid: 100}) {
		t.Fatalf("terminator %+v", got)
	}
	d.m.Channel(2*minute, true)
	if got := d.txs(d.m.Terminator(2*minute, nil)); got[0].f.cto != 1 {
		t.Fatalf("busy terminator %+v", got)
	}
	u := newTiming(t, 100, 2, 1)
	u.rx(minute, ctf{gen: 1, ldi: 3, lid: 7, cto: 1, sid: 55})
	u.state(csbk.TimingLeaderAndTimingUnknown)
	u.leader(0, 0)
	if got := u.txs(u.m.Terminator(minute, nil)); got[0].f != (ctf{sid: 100}) {
		t.Fatalf("terminator without a leader %+v", got)
	}
}

// ETSI TS 102 361-2 clauses 6.2.2.3.2 and 6.2.3.11
func TestTimingPropagation(t *testing.T) {
	t.Parallel()

	d := newTiming(t, 100, 2, 5)
	out := d.rx(minute, beaconOf(3, 7))
	if timings(out) != 1 {
		t.Fatalf("outputs %v", kinds(out))
	}
	d.state(csbk.TimingLeaderAndTimingKnown)
	got := d.txs(d.until(minute + rhotMax + time.Second))
	if len(got) != 1 || got[0].n != 10 || got[0].f.cto != 3 || got[0].f.gen != 1 || got[0].f.ldi != 3 || got[0].f.lid != 7 || got[0].f.sdi != 0 || got[0].f.sid != 100 {
		t.Fatalf("propagation %+v", got)
	}
	if sa := got[0].f.sa; sa > uint16(rhotMax/constants.SyncAgeIncrement)+1 {
		t.Fatalf("sync age %d", sa)
	}
	at := 2 * minute
	for range 3 {
		d.rx(at, beaconOf(3, 7))
		next, ok := d.m.Deadline()
		if !ok || next < at+2160*time.Millisecond || next > at+rhotMax {
			t.Fatalf("CT_RHOT after a propagation: %v", next-at)
		}
		d.rx(at+100*time.Millisecond, ctf{gen: 1, ldi: 3, lid: 7, cto: 3, sid: 31})
		if got := d.txs(d.until(at + 4*time.Second)); len(got) != 0 {
			t.Fatalf("duplicate propagation sent: %+v", got)
		}
		at += minute
	}
	d.rx(at, beaconOf(3, 7))
	if next, _ := d.m.Deadline(); next < at+1800*time.Millisecond || next > at+2880*time.Millisecond {
		t.Fatalf("CT_RHOT after three cancels: %v", next-at)
	}
}

// ETSI TS 102 361-2 clauses 6.2.3.4 and 6.2.3.3
func TestTimingSyncAge(t *testing.T) {
	t.Parallel()

	d := following(t)
	start := minute - 20*constants.SyncAgeIncrement
	got := d.txs(d.until(start + constants.SyncAgeWarning + rhotMax + time.Second))
	if len(got) != 1 || got[0].first < start+constants.SyncAgeWarning || got[0].f.cto != 2 || got[0].f.sdi != 0 || got[0].f.lid != 7 || got[0].n != 3 {
		t.Fatalf("warning request %+v", got)
	}
	d.until(start + constants.SyncAge - time.Millisecond)
	d.state(csbk.TimingLeaderAndTimingKnown)
	d.until(start + constants.SyncAge)
	d.state(csbk.TimingLeaderUnknown)
	d.leader(0, 0)
	if got := d.txs(d.m.Terminator(start+constants.SyncAge, nil)); got[0].f != (ctf{sdi: 2, sid: 100}) {
		t.Fatalf("terminator %+v", got)
	}
	got = d.txs(d.until(start + constants.SyncAge + constants.NoLeader + rhotMax + time.Second))
	if len(got) != 1 || got[0].f != (ctf{sdi: 2, sid: 100}) {
		t.Fatalf("no leader request %+v", got)
	}
}

// ETSI TS 102 361-2 clause 6.2.3.4
func TestTimingWarningRequestKept(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rx   ctf
		keep bool
	}{
		{name: "unaligned", rx: ctf{ldi: 3, lid: 9, cto: 1, sid: 8}, keep: true},
		{name: "lower leader", rx: ctf{gen: 1, ldi: 2, lid: 9, cto: 2, sid: 8}, keep: true},
		{name: "same leader older", rx: ctf{gen: 1, sa: 1500, ldi: 3, lid: 7, cto: 2, sid: 8}, keep: true},
		{name: "same leader same age", rx: ctf{gen: 3, sa: 1080, ldi: 3, lid: 7, cto: 2, sid: 8}},
		{name: "higher leader", rx: ctf{gen: 1, ldi: 3, lid: 99, cto: 2, sid: 8}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := following(t)
			start := minute - 20*constants.SyncAgeIncrement
			w := start + constants.SyncAgeWarning
			d.until(w)
			d.rx(w+10*time.Millisecond, tc.rx)
			got := d.txs(d.until(w + rhotMax + time.Second))
			sent := false
			for _, g := range got {
				sent = sent || g.f.cto == 2 && g.f.lid == 7 && g.f.sdi == 0 && !g.f.nl && g.n == 3
			}
			if sent != tc.keep {
				t.Fatalf("sent %+v", got)
			}
		})
	}
}

// ETSI TS 102 361-2 clauses 6.2.3.7, 6.2.3.8 and 6.2.3.10
func TestTimingEvaluation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rx     ctf
		want   []ctf
		state  csbk.TimingState
		ldi    uint8
		lid    uint32
		timing int
	}{
		{name: "request answered", rx: ctf{sdi: 3, sid: 300}, want: []ctf{{gen: 2, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7},
		{name: "unaligned lower corrected", rx: ctf{gen: 1, ldi: 2, lid: 9, cto: 1, sid: 300}, want: []ctf{{gen: 2, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7},
		{name: "unaligned higher ignored", rx: ctf{gen: 1, ldi: 3, lid: 99, cto: 1, sid: 300}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7},
		{name: "lower leader corrected", rx: ctf{gen: 1, ldi: 2, lid: 9, cto: 2, sid: 300}, want: []ctf{{gen: 2, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7},
		{name: "older age corrected", rx: ctf{gen: 1, sa: 900, ldi: 3, lid: 7, cto: 2, sid: 300}, want: []ctf{{gen: 2, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7},
		{name: "higher leader status", rx: ctf{gen: 4, sa: 5, ldi: 3, lid: 99, cto: 2, sid: 300}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 99, timing: 1},
		{name: "higher leader push", rx: ctf{gen: 4, sa: 5, ldi: 3, lid: 99, cto: 3, sid: 300}, want: []ctf{{gen: 5, sa: 5, ldi: 3, lid: 99, cto: 3, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 99, timing: 1},
		{name: "same leader shorter path", rx: ctf{gen: 0, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 300}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7, timing: 1},
		{name: "same leader shorter path push", rx: ctf{gen: 0, sa: 20, ldi: 3, lid: 7, cto: 3, sid: 300}, want: []ctf{{gen: 1, sa: 20, ldi: 3, lid: 7, cto: 3, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7, timing: 1},
		{name: "same leader longer path", rx: ctf{gen: 6, sa: 20, ldi: 3, lid: 7, cto: 3, sid: 300}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7},
		{name: "same leader fresher", rx: ctf{gen: 2, sa: 2, ldi: 3, lid: 7, cto: 3, sid: 300}, want: []ctf{{gen: 3, sa: 2, ldi: 3, lid: 7, cto: 3, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 7, timing: 1},
		{name: "appointed", rx: ctf{gen: 1, sa: 40, ldi: 2, lid: 100, nl: true, cto: 2, sid: 300}, want: []ctf{{ldi: 2, lid: 100, cto: 3, sdi: 2, sid: 100}}, state: csbk.TimingLeader, ldi: 2, lid: 100, timing: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := following(t)
			out := d.rx(minute, tc.rx)
			if n := timings(out); n != tc.timing {
				t.Fatalf("%d timing outputs", n)
			}
			got := d.txs(d.until(minute + rhotMax + time.Second))
			d.state(tc.state)
			d.leader(tc.ldi, tc.lid)
			if len(got) != len(tc.want) {
				t.Fatalf("sent %+v", got)
			}
			for i := range got {
				w := tc.want[i]
				if g := aged(got[i], minute); g != w {
					t.Fatalf("sent %+v, want %+v", g, w)
				}
				n := 3
				if w.cto == 3 {
					n = 10
				}
				if got[i].n != n {
					t.Fatalf("%d bursts", got[i].n)
				}
			}
		})
	}
}

// ETSI TS 102 361-2 clause 6.2.3.7
func TestTimingNoLeaderEvaluation(t *testing.T) {
	t.Parallel()

	a := newTiming(t, 100, 2, 1)
	a.rx(minute, ctf{sdi: 3, sid: 400})
	got := a.txs(a.until(minute + rhotMax + time.Second))
	if len(got) != 1 || got[0].f != (ctf{gen: 1, ldi: 3, lid: 400, nl: true, cto: 2, sid: 100}) {
		t.Fatalf("appoint %+v", got)
	}
	a.state(csbk.TimingLeaderAndTimingKnown)
	b := newTiming(t, 100, 2, 1)
	b.rx(minute, ctf{sdi: 2, sid: 400})
	got = b.txs(b.until(minute + rhotMax + time.Second))
	if len(got) != 1 || got[0].f != (ctf{sdi: 2, sid: 100}) {
		t.Fatalf("request %+v", got)
	}
	b.state(csbk.TimingLeaderAndTimingUnknown)
	c := newTiming(t, 100, 2, 1)
	c.rx(minute, ctf{gen: 3, ldi: 2, lid: 7, cto: 0, sid: 400})
	c.state(csbk.TimingLeaderAndTimingUnknown)
}

// ETSI TS 102 361-2 clause 6.2.3.7
func TestTimingNewID(t *testing.T) {
	t.Parallel()

	d := following(t)
	d.rx(minute, ctf{gen: 1, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 100})
	got := d.txs(d.m.Terminator(minute, nil))
	if got[0].f.sid == 100 || got[0].f.sid == 0 || got[0].f.sid > csbk.MaxTimingID {
		t.Fatalf("source id %d", got[0].f.sid)
	}
	e := following(t)
	e.rx(minute, ctf{gen: 1, sa: 20, ldi: 2, lid: 100, cto: 2, sdi: 3, sid: 9})
	if got := e.txs(e.m.Terminator(minute, nil)); got[0].f.sid == 100 {
		t.Fatal("leader id equal to own id kept")
	}
}

func aged(x tx, at time.Duration) ctf {
	f := x.f
	f.sa -= uint16((x.first - at) / constants.SyncAgeIncrement)
	return f
}

// ETSI TS 102 361-2 clause 6.2.3.10
func TestTimingFollowerAppoints(t *testing.T) {
	t.Parallel()

	d := newTiming(t, 100, 2, 1)
	d.rx(minute, ctf{gen: 1, sa: 20, ldi: 1, lid: 7, cto: 2, sid: 55})
	d.leader(1, 7)
	d.rx(minute, ctf{sdi: 3, sid: 300})
	got := d.txs(d.until(minute + rhotMax + time.Second))
	d.leader(3, 300)
	if len(got) != 1 || aged(got[0], minute) != (ctf{gen: 1, sa: 20, ldi: 3, lid: 300, nl: true, cto: 2, sid: 100}) {
		t.Fatalf("appoint %+v", got)
	}
}

func appointed(t *testing.T) *tdriver {
	t.Helper()
	d := newTiming(t, 100, 2, 7)
	d.rx(minute, ctf{gen: 1, sa: 40, ldi: 2, lid: 100, nl: true, cto: 2, sid: 300})
	d.state(csbk.TimingLeader)
	got := d.txs(d.until(minute + time.Second))
	if len(got) != 1 || got[0].first != minute || got[0].n != 10 || got[0].f != beaconOf(2, 100) {
		t.Fatalf("beacon %+v", got)
	}
	return d
}

// ETSI TS 102 361-2 clause 6.2.3.5
func TestTimingLeaderBeacon(t *testing.T) {
	t.Parallel()

	d := appointed(t)
	got := d.txs(d.until(minute + 2*constants.BeaconInterval + rhotMax + time.Second))
	if len(got) != 2 || got[0].first < minute+constants.BeaconInterval || got[0].first > minute+constants.BeaconInterval+rhotMax || got[1].first < minute+2*constants.BeaconInterval {
		t.Fatalf("beacons %+v", got)
	}
	for _, g := range got {
		if g.f != beaconOf(2, 100) || g.n != 10 {
			t.Fatalf("beacon %+v", g)
		}
	}
	if got := d.txs(d.m.Terminator(got[1].first+minute, nil)); got[0].f != (ctf{sa: 120, ldi: 2, lid: 100, cto: 2, sdi: 2, sid: 100}) {
		t.Fatalf("terminator %+v", got)
	}
}

// ETSI TS 102 361-2 clauses 6.2.3.5, 6.2.3.6, 6.2.3.9 and 6.2.3.10
func TestTimingLeaderEvaluation(t *testing.T) {
	t.Parallel()

	resp := ctf{sa: 20, ldi: 2, lid: 100, cto: 2, sdi: 2, sid: 100}
	tests := []struct {
		name  string
		rx    ctf
		want  []ctf
		state csbk.TimingState
		ldi   uint8
		lid   uint32
	}{
		{name: "lower leader corrected", rx: ctf{gen: 1, ldi: 1, lid: 9, cto: 2, sid: 8}, want: []ctf{resp}, state: csbk.TimingLeader, ldi: 2, lid: 100},
		{name: "request answered", rx: ctf{sdi: 1, sid: 8}, want: []ctf{resp}, state: csbk.TimingLeader, ldi: 2, lid: 100},
		{name: "request appoints", rx: ctf{sdi: 3, sid: 8}, want: []ctf{{sa: 20, ldi: 3, lid: 8, nl: true, gen: 1, cto: 2, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 8},
		{name: "higher leader accepted", rx: ctf{gen: 2, sa: 6, ldi: 3, lid: 9, cto: 2, sdi: 1, sid: 8}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 9},
		{name: "higher leader appoints source", rx: ctf{gen: 2, sa: 6, ldi: 2, lid: 900, cto: 2, sdi: 3, sid: 8}, want: []ctf{{gen: 1, sa: 20, ldi: 3, lid: 8, nl: true, cto: 2, sid: 100}}, state: csbk.TimingLeaderAndTimingKnown, ldi: 3, lid: 8},
		{name: "own beacon relayed", rx: ctf{gen: 1, sa: 25, ldi: 2, lid: 100, cto: 3, sid: 8}, state: csbk.TimingLeader, ldi: 2, lid: 100},
		{name: "own beacon of an earlier interval", rx: ctf{gen: 1, sa: 560, ldi: 2, lid: 100, cto: 3, sid: 8}, want: []ctf{resp}, state: csbk.TimingLeader, ldi: 2, lid: 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := appointed(t)
			at := minute + 10*time.Second
			d.rx(at, tc.rx)
			got := d.txs(d.until(at + rhotMax + time.Second))
			d.state(tc.state)
			d.leader(tc.ldi, tc.lid)
			if len(got) != len(tc.want) {
				t.Fatalf("sent %+v", got)
			}
			for i := range got {
				if aged(got[i], at) != tc.want[i] || got[i].n != 3 {
					t.Fatalf("sent %+v, want %+v", got[i], tc.want[i])
				}
			}
		})
	}
}

// ETSI TS 102 361-2 clause 6.2.3.6
func TestTimingLeaderConflict(t *testing.T) {
	t.Parallel()

	stays, accepts := 0, 0
	for seed := range uint64(40) {
		d := newTiming(t, 0x80000, 2, seed)
		d.rx(minute, ctf{gen: 1, sa: 40, ldi: 2, lid: 0x80000, nl: true, cto: 2, sid: 300})
		d.until(minute + time.Second)
		d.rx(minute+10*time.Second, ctf{gen: 1, sa: 300, ldi: 2, lid: 0x80000, cto: 3, sid: 8})
		_, id, _ := d.m.Leader()
		switch d.m.State() {
		case csbk.TimingLeader:
			if id <= 0x80000 {
				t.Fatalf("seed %d: stays leader with id %#x", seed, id)
			}
			stays++
		case csbk.TimingLeaderAndTimingKnown:
			if id != 0x80000 {
				t.Fatalf("seed %d: leader %#x", seed, id)
			}
			accepts++
		default:
			t.Fatalf("seed %d: state %v", seed, d.m.State())
		}
	}
	if stays == 0 || accepts == 0 {
		t.Fatalf("stays %d, accepts %d", stays, accepts)
	}
}

// ETSI TS 102 361-2 clause 6.2.3.7, Figure 6.7
func TestTimingLeaderSyncAgeWrap(t *testing.T) {
	t.Parallel()

	for _, sa := range []uint16{1080 + 535, 540 + 9, 2 * 540} {
		d := newTiming(t, 0x80000, 2, 1)
		d.rx(minute, ctf{gen: 1, sa: 0, ldi: 2, lid: 0x80000, nl: true, cto: 2, sid: 300})
		d.until(minute + time.Second)
		d.rx(minute+time.Second, ctf{gen: 1, sa: sa, ldi: 2, lid: 0x80000, cto: 3, sid: 8})
		if _, id, _ := d.m.Leader(); d.m.State() != csbk.TimingLeader || id != 0x80000 {
			t.Fatalf("SA %d: state %v, leader %#x", sa, d.m.State(), id)
		}
	}
}

// ETSI TS 102 361-2 clause 6.2.3.8
func TestTimingBusyChannel(t *testing.T) {
	t.Parallel()

	d := following(t)
	d.rx(minute, ctf{sdi: 1, sid: 300})
	d.m.Channel(minute, true)
	if got := d.txs(d.until(minute + time.Minute)); len(got) != 0 {
		t.Fatalf("sent on a busy channel: %+v", got)
	}
	d.m.Channel(2*minute, false)
	if got := d.txs(d.until(2*minute + rhotMax + time.Second)); len(got) != 1 {
		t.Fatalf("not sent on an idle channel: %+v", got)
	}
	d.rx(3*minute, ctf{sdi: 1, sid: 300})
	d.m.Channel(3*minute, true)
	d.until(5*minute + rhotMax)
	d.m.Channel(5*minute+rhotMax, false)
	if got := d.txs(d.until(6 * minute)); len(got) != 0 {
		t.Fatalf("sent after two minutes: %+v", got)
	}
}

// ETSI TS 102 361-2 clauses 6.2.1 and 6.2.3
func TestTimingElection(t *testing.T) {
	t.Parallel()

	radios := []*tdriver{newTiming(t, 11, 1, 1), newTiming(t, 22, 3, 2), newTiming(t, 33, 2, 3), newTiming(t, 44, 0, 4)}
	for now := time.Duration(0); now < 12*minute; now += 30 * time.Millisecond {
		for i, r := range radios {
			out := r.until(now)
			for j, other := range radios {
				if j == i {
					continue
				}
				for _, o := range out {
					if o.Kind == csbk.OutputBurst {
						other.keep(other.m.Burst(o.Due, &o.Burst, layer2.Hint{}, other.out[:0]))
						other.m.Channel(o.Due, true)
					}
				}
			}
		}
		for _, r := range radios {
			if _, ok := r.m.Deadline(); ok {
				r.m.Channel(now, false)
			}
		}
	}
	radios[1].state(csbk.TimingLeader)
	di, id, _ := radios[1].m.Leader()
	for _, r := range []*tdriver{radios[0], radios[2], radios[3]} {
		r.state(csbk.TimingLeaderAndTimingKnown)
		r.leader(di, id)
	}
	if di != 3 {
		t.Fatalf("leader preference %d", di)
	}
}

func TestTimingConfig(t *testing.T) {
	t.Parallel()

	for _, cfg := range []*csbk.TimingConfig{nil, {}, {ID: csbk.MaxTimingID + 1, Slot: enums.Slot1}, {ID: 1, DynamicID: 4, Slot: enums.Slot1}, {ID: 1}} {
		if _, err := csbk.NewTiming(cfg); err == nil {
			t.Errorf("%+v: no error", cfg)
		}
	}
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: 1, Slot: enums.Slot2})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Deadline(); ok {
		t.Fatal("deadline before Join")
	}
	if out := m.Tick(time.Hour, nil); len(out) != 0 {
		t.Fatal("Tick before Join")
	}
	if out := m.Burst(0, ctBurst(t, beaconOf(3, 7)), layer2.Hint{}, nil); len(out) != 0 || m.State() != csbk.TimingLeaderAndTimingUnknown {
		t.Fatal("Burst before Join")
	}
	m.Join(0)
	if out := m.Decoded(0, nil, layer2.Hint{}, nil); len(out) != 0 {
		t.Fatal("nil burst")
	}
	m.Burst(0, ctBurst(t, beaconOf(3, 7)), layer2.Hint{}, nil)
	m.Reset()
	if _, ok := m.Deadline(); ok || m.State() != csbk.TimingLeaderAndTimingUnknown {
		t.Fatal("Reset")
	}
	names := map[csbk.TimingState]string{csbk.TimingLeaderAndTimingUnknown: "LeaderAndTimingUnknown", csbk.TimingLeaderUnknown: "LeaderUnknown",
		csbk.TimingLeaderAndTimingKnown: "LeaderAndTimingKnown", csbk.TimingLeader: "Leader", 9: "TimingState(9)"}
	for s, want := range names {
		if s.String() != want {
			t.Errorf("%d: %q", s, s.String())
		}
	}
	if csbk.ServiceTiming.String() != "Timing" || csbk.OutputTiming.String() != "Timing" {
		t.Error("names")
	}
}
