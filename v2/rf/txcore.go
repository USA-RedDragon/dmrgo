package rf

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

// ErrBurstOrder is the error of Write on a transmitter for a burst whose
// Slot does not increase within its Stream.
var ErrBurstOrder = errors.New("rf: burst slot does not increase within its stream")

// ErrBurstService is the error of Write on a transmitter of the
// license-free service for a BS-sourced burst: the service allows no base
// station or repeater.
var ErrBurstService = errors.New("rf: BS-sourced burst in the license-free service")

// ErrTxTimeout is the error of Write on a transmitter of the license-free
// service for a burst that would carry one transmission past 179 s. The
// transmitter keys down, and refuses the rest of the Stream; a new Stream
// starts a new transmission.
var ErrTxTimeout = errors.New("rf: transmission past 179 s in the license-free service")

const (
	txGuard     = cachSymbols
	txLead      = cachSymbols / 2
	txTail      = 6
	txSlotSyms  = slotSymbols
	txIdleSlots = 1
	txLFSlots   = 179 * 1000 / 30
	txRampSec   = 1e-3
	txQueue     = 8
)

type txSymbols interface {
	symbol(s int8)
	key(on bool)
}

type txKey struct {
	at int64
	on bool
}

type txCore struct {
	syms     [burstSymbols]int8
	q        [txQueue]txKey
	n        int64
	slot     int64
	stream   int
	dead     int
	lag      int32
	keyed    int32
	qn       uint8
	open     bool
	carrier  bool
	lf       bool
	timedOut bool
}

func (c *txCore) reset(lag int, lf bool) {
	*c = txCore{}
	c.lag, c.lf = int32(lag), lf
}

func (c *txCore) feed(o txSymbols, s int8) {
	for c.qn > 0 && c.q[0].at <= c.n {
		o.key(c.q[0].on)
		copy(c.q[:c.qn-1], c.q[1:c.qn])
		c.qn--
	}
	o.symbol(s)
	c.n++
}

func (c *txCore) setKey(o txSymbols, on bool) {
	if c.carrier == on {
		return
	}
	c.carrier = on
	if c.qn == txQueue {
		o.key(c.q[0].on)
		copy(c.q[:txQueue-1], c.q[1:])
		c.qn--
	}
	c.q[c.qn] = txKey{at: c.n + int64(c.lag), on: on}
	c.qn++
}

func (c *txCore) zeros(o txSymbols, k int) {
	for range k {
		c.feed(o, 0)
	}
}

func (c *txCore) bits(o txSymbols, b []byte) {
	for _, s := range fsktx.Dibits(c.syms[:], b) {
		c.feed(o, s)
	}
}

func (c *txCore) down(o txSymbols) {
	if !c.open {
		return
	}
	c.setKey(o, false)
	c.zeros(o, int(c.lag)+txTail)
	c.open = false
}

func (c *txCore) put(o txSymbols, b *Burst) error {
	if c.lf && b.BS {
		return ErrBurstService
	}
	if c.timedOut && b.Stream == c.dead {
		return ErrTxTimeout
	}
	c.timedOut = false
	gap := int64(0)
	if c.open && b.Stream == c.stream {
		if b.Slot <= c.slot {
			return ErrBurstOrder
		}
		gap = b.Slot - c.slot - 1
	}
	if c.open && (b.Stream != c.stream || gap > txIdleSlots) {
		c.down(o)
	}
	if c.open {
		if c.lf && int64(c.keyed)+gap+1 > txLFSlots {
			c.down(o)
			c.timedOut, c.dead = true, b.Stream
			return ErrTxTimeout
		}
		c.keyed += int32(gap) + 1
		if gap == 0 && b.BS {
			c.slotBody(o, b)
			c.slot = b.Slot
			return nil
		}
		c.setKey(o, false)
		if gap > 0 {
			if b.BS {
				c.zeros(o, txSlotSyms-txLead)
				c.setKey(o, true)
				c.zeros(o, txLead)
			} else {
				c.zeros(o, txSlotSyms)
			}
		}
	} else {
		c.open, c.stream, c.keyed = true, b.Stream, 1
		if b.BS {
			c.setKey(o, true)
			c.zeros(o, txLead)
		}
	}
	c.slotBody(o, b)
	c.slot = b.Slot
	return nil
}

func (c *txCore) slotBody(o txSymbols, b *Burst) {
	data, cach := &b.Data, &b.CACH
	if b.HasFixed {
		data, cach = &b.Fixed, &b.FixedCACH
	}
	if b.BS {
		c.setKey(o, true)
		if b.HasCACH {
			c.bits(o, cach[:])
		} else {
			c.zeros(o, txGuard)
		}
	} else {
		c.zeros(o, txLead)
		c.setKey(o, true)
		c.zeros(o, txGuard-txLead)
	}
	c.bits(o, data[:])
}

func txMaxSymbols(n, lag int) int64 {
	tail := int64(lag + txTail)
	if n <= 0 {
		return tail
	}
	per := int64(2*txSlotSyms) + tail
	return min(int64(n), 1<<30)*per + tail
}
