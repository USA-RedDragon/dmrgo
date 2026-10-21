package pdu

import "github.com/USA-RedDragon/dmrgo/v2"

// CapacityPlusMaxBlocks is the most CSBK blocks of one Capacity Plus site
// status that CapacityPlusSiteStatusAssembler joins: enough for 16 busy
// talkgroup LSNs and 16 busy radio LSNs.
const CapacityPlusMaxBlocks = 8

const capacityPlusBlockOctets = 7

// CapacityPlusActivity is the channel activity of a Capacity Plus site
// status: the rest LSN, and per LSN the talkgroup of a group call or the
// radio of a private voice or data call. Bit n-1 of a bitmap is LSN n.
type CapacityPlusActivity struct {
	// RestLSN is the rest LSN of the first block.
	RestLSN uint8
	// Voice is the bitmap of the LSNs with a group call.
	Voice uint16
	// Talkgroups holds the 8-bit talkgroup of each LSN in Voice, at index
	// LSN-1.
	Talkgroups [CapacityPlusMaxLSN]uint8
	// Options is the octet after the talkgroups. Bit 7 set means that the
	// radio bitmaps follow; the meaning of the other bits is unknown.
	Options uint8
	// Radio is the bitmap of the LSNs with a private voice or data call.
	// The site status does not tell the two apart.
	Radio uint16
	// Radios holds the 16-bit target radio of each LSN in Radio, at index
	// LSN-1.
	Radios [CapacityPlusMaxLSN]uint16
}

// Talkgroup returns the talkgroup of a group call on lsn. ok is false
// when lsn has no group call or is out of range.
func (a *CapacityPlusActivity) Talkgroup(lsn uint8) (tg uint8, ok bool) {
	if a == nil {
		return 0, false
	}
	if lsn == 0 || lsn > CapacityPlusMaxLSN || a.Voice&(1<<(lsn-1)) == 0 {
		return 0, false
	}
	return a.Talkgroups[lsn-1], true
}

// RadioTarget returns the target radio of a private voice or data call on
// lsn. ok is false when lsn has no such call or is out of range.
func (a *CapacityPlusActivity) RadioTarget(lsn uint8) (radio uint16, ok bool) {
	if a == nil {
		return 0, false
	}
	if lsn == 0 || lsn > CapacityPlusMaxLSN || a.Radio&(1<<(lsn-1)) == 0 {
		return 0, false
	}
	return a.Radios[lsn-1], true
}

// CapacityPlusSiteStatusAssembler joins the blocks of a Capacity Plus site
// status and decodes its activity. Each timeslot sends its own status, so
// a receiver of both timeslots keeps one assembler per timeslot. The zero
// CapacityPlusSiteStatusAssembler is ready to use.
type CapacityPlusSiteStatusAssembler struct {
	stream [CapacityPlusMaxBlocks * capacityPlusBlockOctets]byte
	blocks uint8
	rest   uint8
	open   bool
	done   bool
}

// Reset drops the partial or complete status.
func (a *CapacityPlusSiteStatusAssembler) Reset() {
	if a == nil {
		return
	}
	*a = CapacityPlusSiteStatusAssembler{}
}

// Add adds a block. It returns true when the block completes a status,
// that is for a single or last block. A first or single block starts a new
// status. It returns ErrCapacityPlusSequence for a continuation or last
// block without a first block, and for a block past
// CapacityPlusMaxBlocks. A nil p does nothing.
func (a *CapacityPlusSiteStatusAssembler) Add(p *CapacityPlusSiteStatusPDU) (bool, error) {
	if a == nil {
		return false, dmrgo.ErrNilReceiver
	}
	if p == nil {
		return false, nil
	}
	switch p.Segment {
	case CapacityPlusSegmentFirst, CapacityPlusSegmentSingle:
		a.Reset()
		a.rest, a.open = p.RestLSN, true
	default:
		if !a.open || a.blocks >= CapacityPlusMaxBlocks {
			a.Reset()
			return false, ErrCapacityPlusSequence
		}
	}
	copy(a.stream[int(a.blocks)*capacityPlusBlockOctets:], p.Activity[:])
	a.blocks++
	if p.Segment == CapacityPlusSegmentSingle || p.Segment == CapacityPlusSegmentLast {
		a.open, a.done = false, true
		return true, nil
	}
	a.done = false
	return false, nil
}

// Activity decodes the last complete status. It returns
// ErrCapacityPlusIncomplete when no status is complete, and when a bitmap
// names more talkgroups or radios than the status holds; the activity then
// has the entries that the status holds.
func (a *CapacityPlusSiteStatusAssembler) Activity() (CapacityPlusActivity, error) {
	if a == nil {
		return CapacityPlusActivity{}, dmrgo.ErrNilReceiver
	}
	act := CapacityPlusActivity{RestLSN: a.rest}
	if !a.done {
		return act, ErrCapacityPlusIncomplete
	}
	r := capacityPlusReader{b: &a.stream, n: int(a.blocks) * capacityPlusBlockOctets}
	if !r.bank(&act.Voice, 0, &act.Talkgroups, nil) || !r.bank(&act.Voice, 8, &act.Talkgroups, nil) {
		return act, ErrCapacityPlusIncomplete
	}
	opts, ok := r.next()
	if !ok {
		return act, ErrCapacityPlusIncomplete
	}
	act.Options = opts
	if opts&0x80 == 0 {
		return act, nil
	}
	if !r.bank(&act.Radio, 0, nil, &act.Radios) || !r.bank(&act.Radio, 8, nil, &act.Radios) {
		return act, ErrCapacityPlusIncomplete
	}
	return act, nil
}

type capacityPlusReader struct {
	b   *[CapacityPlusMaxBlocks * capacityPlusBlockOctets]byte
	n   int
	off int
}

func (r *capacityPlusReader) next() (byte, bool) {
	if r.off >= r.n {
		return 0, false
	}
	v := r.b[r.off]
	r.off++
	return v, true
}

func (r *capacityPlusReader) bank(bitmap *uint16, base int, tgs *[CapacityPlusMaxLSN]uint8, radios *[CapacityPlusMaxLSN]uint16) bool {
	flags, ok := r.next()
	if !ok {
		return false
	}
	for i := range 8 {
		if flags&(0x80>>i) == 0 {
			continue
		}
		hi, ok := r.next()
		if !ok {
			return false
		}
		if radios != nil {
			lo, ok := r.next()
			if !ok {
				return false
			}
			radios[base+i] = uint16(hi)<<8 | uint16(lo)
		} else {
			tgs[base+i] = hi
		}
		*bitmap |= 1 << (base + i)
	}
	return true
}
