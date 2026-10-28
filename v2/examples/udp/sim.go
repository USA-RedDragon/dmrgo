package main

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

const (
	unitA    = 3120001
	unitB    = 3120002
	dictID   = 1
	dictSize = 4096
	dictSeed = 1000
)

type cell struct {
	rate  enums.DataType
	ber   float64
	enc   encoding
	forms int
	seed  uint64
}

type cellResult struct {
	cell
	tx, rx  appStats
	air     airStats
	link    linkStats
	elapsed time.Duration
	sent    []form
}

var errStuck = errors.New("simulation did not finish")

func runCell(c cell, t *template, dict []byte) (cellResult, error) {
	res := cellResult{cell: c}
	ca, err := newCodec(dictID, dict)
	if err != nil {
		return res, err
	}
	defer ca.close()
	cb, err := newCodec(dictID, dict)
	if err != nil {
		return res, err
	}
	defer cb.close()
	a := &air{rng: newRand(c.seed), ber: c.ber}
	sa := &station{peer: unitB, codec: ca, tmpl: t, enc: c.enc}
	sb := &station{peer: unitA, codec: cb, tmpl: t, enc: c.enc}
	if sa.l, err = newLink(unitA, c.rate, c.seed+1, sa, a.emit(0)); err != nil {
		return res, err
	}
	if sb.l, err = newLink(unitB, c.rate, c.seed+2, sb, a.emit(1)); err != nil {
		return res, err
	}
	a.links = [2]*link{sa.l, sb.l}
	rng := newRand(c.seed + 3)
	limit := time.Duration(c.forms) * 10 * time.Minute
	now := time.Duration(0)
	for {
		if sa.idle() && len(res.sent) < c.forms {
			f := randomForm(rng, t)
			res.sent = append(res.sent, f)
			sa.submit(now, &f)
		}
		next, ok := time.Duration(0), false
		for _, d := range []func() (time.Duration, bool){a.deadline, sa.l.deadline, sb.l.deadline, sa.deadline} {
			if at, dok := d(); dok && (!ok || at < next) {
				next, ok = at, true
			}
		}
		if !ok {
			if sa.idle() && len(res.sent) == c.forms {
				break
			}
			return res, fmt.Errorf("%w: no deadline at %v", errStuck, now)
		}
		now = max(now, next)
		if now > limit {
			return res, fmt.Errorf("%w: past %v", errStuck, limit)
		}
		a.tick(now)
		sa.l.tick(now)
		sb.l.tick(now)
		sa.tick(now)
	}
	res.tx, res.rx, res.air, res.link, res.elapsed = sa.stats, sb.stats, a.stats, sa.l.stats, now
	if !sameForms(res.sent, sb.got, res.tx.acked) {
		return res, errors.New("delivered forms do not match the sent forms")
	}
	return res, nil
}

func sameForms(sent, got []form, acked int) bool {
	if len(got) < acked || len(got) > len(sent) {
		return false
	}
	j := 0
	for i := range got {
		for j < len(sent) && !slices.Equal(sent[j].values, got[i].values) {
			j++
		}
		if j == len(sent) {
			return false
		}
		j++
	}
	return true
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
