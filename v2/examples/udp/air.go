package main

import (
	"math/rand/v2"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const (
	slotPeriod = 60 * time.Millisecond
	burstTime  = 30 * time.Millisecond
	busyHang   = 2 * slotPeriod
	burstBits  = 264
)

func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) //nolint:gosec // a simulation needs a seeded generator
}

type flight struct {
	at   time.Duration
	from int
	raw  layer2.RawBurst
	lost bool
}

type airStats struct {
	bursts, collisions int
	busy               time.Duration
}

type air struct {
	rng       *rand.Rand
	ber       float64
	links     [2]*link
	flights   []flight
	recent    []flight
	busyUntil [2]time.Duration
	busy      [2]bool
	stats     airStats
}

func (a *air) emit(from int) func(time.Duration, *layer2.RawBurst) {
	return func(at time.Duration, raw *layer2.RawBurst) {
		f := flight{at: at, from: from, raw: *raw}
		for _, list := range [][]flight{a.flights, a.recent} {
			for i := range list {
				o := &list[i]
				if o.from != from && o.at-at < burstTime && at-o.at < burstTime {
					o.lost, f.lost = true, true
				}
			}
		}
		if f.lost {
			a.stats.collisions++
		}
		a.stats.bursts++
		a.stats.busy += burstTime
		a.flights = append(a.flights, f)
	}
}

func (a *air) deadline() (time.Duration, bool) {
	var at time.Duration
	ok := false
	for i := range a.flights {
		if !ok || a.flights[i].at < at {
			at, ok = a.flights[i].at, true
		}
	}
	for k := range a.busy {
		if a.busy[k] && (!ok || a.busyUntil[k] < at) {
			at, ok = a.busyUntil[k], true
		}
	}
	return at, ok
}

func (a *air) tick(now time.Duration) {
	keep := a.recent[:0]
	for _, f := range a.recent {
		if now-f.at < slotPeriod {
			keep = append(keep, f)
		}
	}
	a.recent = keep
	for i := 0; i < len(a.flights); {
		f := a.flights[i]
		if f.at > now {
			i++
			continue
		}
		a.flights = append(a.flights[:i], a.flights[i+1:]...)
		a.recent = append(a.recent, f)
		to := 1 - f.from
		a.hold(to, f.at)
		if f.lost {
			continue
		}
		a.flip(&f.raw)
		a.links[to].hear(now, &f.raw)
	}
	for k := range a.busy {
		if a.busy[k] && a.busyUntil[k] <= now {
			a.busy[k] = false
			a.links[k].channel(now, false)
		}
	}
}

func (a *air) hold(k int, at time.Duration) {
	a.busyUntil[k] = at + busyHang
	if !a.busy[k] {
		a.busy[k] = true
		a.links[k].channel(at, true)
	}
}

func (a *air) flip(raw *layer2.RawBurst) {
	if a.ber <= 0 {
		return
	}
	for b := range burstBits {
		if a.rng.Float64() < a.ber {
			raw[b/8] ^= byte(1) << (7 - b%8)
		}
	}
}
