package rf

import (
	"errors"
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ErrNilBurst is the error of Monitor.Burst for a nil Monitor, Burst or
// layer2.Burst.
var ErrNilBurst = errors.New("rf: nil Monitor or Burst")

// Hint returns what the Receiver knows about b besides its bits, for a
// machine that takes a layer2.RawBurst with a layer2.Hint. Slot is the TDMA
// channel from the TACT of the CACH of b, when b is BS-sourced and its TACT
// decodes, and Kind and VoiceBurst are set when the Receiver placed a voice
// burst by timing. A Monitor gives Slot from the CACHs of the whole stream,
// which one bad CACH does not change. A nil b gives the zero Hint.
func (b *Burst) Hint() layer2.Hint {
	if b == nil {
		return layer2.Hint{}
	}

	var h layer2.Hint
	if b == nil {
		return h
	}
	if b.Voice != enums.VoiceBurstUnknown {
		h.Kind, h.VoiceBurst = layer2.KindVoice, b.Voice
	}
	if b.BS && b.HasCACH {
		_, cach := b.Decoded()
		if c := layer2.DecodeCACHField(cach); !c.TACT.FEC.Uncorrectable {
			h.Slot = enums.Slot1
			if c.TACT.TDMAChannel {
				h.Slot = enums.Slot2
			}
		}
	}
	return h
}

// Monitor follows the TDMA channels of the bursts of one Receiver or
// DiscReceiver, as a scanner does. It finds the TDMA channel of each
// BS-sourced burst from the CACHs of its stream, decodes the burst, keeps
// the superframe position and the embedded LC of each channel, and keeps
// the LC of the call on each channel from its voice LC header, embedded LC
// and terminator. The zero Monitor is ready to use, and it does not
// allocate. A Monitor is not safe for concurrent use.
type Monitor struct {
	opts   layer2.DecodeOptions
	ch     [3]monitorChannel
	t0     float64
	ref    bool
	stream int
	votes  [2]int
}

type monitorChannel struct {
	voice layer2.VoiceReceiver
	flc   pdu.FullLinkControl
	p     pdu.FullLinkControlPayloads
	lc    pdu.LCBytes
	hasLC bool
}

// MonitorBurst is what a Monitor finds in one Burst.
type MonitorBurst struct {
	// Hint is what the Monitor knows about the burst besides its bits:
	// Slot is the TDMA channel from the CACHs of the stream, or
	// enums.SlotUnknown for a burst without a CACH, and Kind and VoiceBurst
	// come from the timing, as Burst.Hint gives them.
	Hint layer2.Hint
	// LC is the LC of the call on the TDMA channel, when HasLC.
	LC pdu.LCBytes
	// HasLC reports that LC is set.
	HasLC bool
	// LCDone reports that this burst completed a Full LC: a voice LC
	// header, a terminator with LC, or the embedded LC of a superframe.
	LCDone bool
	// End reports a terminator with LC: the call on the channel ended, and
	// the Monitor forgets its LC after this burst.
	End bool
}

// Reset forgets the streams and the calls of every channel. Call it after
// Flush of the receiver, when the next samples are of another recording.
func (m *Monitor) Reset() {
	if m == nil {
		return
	}
	for i := range m.ch {
		m.ch[i].voice.Reset()
		m.ch[i].hasLC = false
	}
	m.ref, m.stream, m.votes = false, 0, [2]int{}
}

// Burst decodes b into dst, finds its TDMA channel and updates the call of
// the channel. Make dst with layer2.NewBurst, so that no decode allocates,
// and pass the decoded dst to the Decoded method of each machine of the
// channel. Burst returns ErrNilBurst for a nil m, dst or b, and the error of
// layer2.Burst.DecodeFromBytesWith, with dst and Hint set but the call of
// the channel unchanged.
func (m *Monitor) Burst(dst *layer2.Burst, b *Burst) (MonitorBurst, error) {
	if m == nil || dst == nil || b == nil {
		return MonitorBurst{}, ErrNilBurst
	}
	data, cach := b.Decoded()
	h := b.Hint()
	h.Slot = m.slot(b, cach)
	out := MonitorBurst{Hint: h}
	m.opts = layer2.DecodeOptions{}
	if b.Voice != enums.VoiceBurstUnknown {
		m.opts.Kind, m.opts.VoiceBurst = layer2.KindVoice, b.Voice
	} else if b.Distance > layer2.DefaultSyncThreshold {
		m.opts.SyncThreshold = b.Distance
	}
	lb := dst
	if err := lb.DecodeFromBytesWith(data, &m.opts); err != nil {
		return out, err
	}
	ch := &m.ch[h.Slot]
	if ok, _ := ch.voice.ReceiveInto(&ch.flc, &ch.p, lb); ok {
		ch.set(&ch.flc)
		out.LCDone = true
	}
	if b.Voice != enums.VoiceBurstUnknown {
		lb.VoiceBurst = b.Voice
	}
	if lb.Kind == layer2.KindData {
		if f, ok := lb.Data.(*pdu.FullLinkControl); ok && !f.FEC.Uncorrectable {
			ch.set(f)
			out.LCDone = true
			out.End = f.BurstDataType == enums.DataTypeTerminatorWithLC
		}
	}
	out.LC, out.HasLC = ch.lc, ch.hasLC
	if out.End {
		ch.hasLC = false
	}
	return out, nil
}

func (c *monitorChannel) set(f *pdu.FullLinkControl) {
	lc, err := f.Bytes()
	_, _, ok := f.Addresses()
	c.lc, c.hasLC = lc, err == nil && ok
}

func (m *Monitor) slot(b *Burst, cach [3]byte) enums.Slot {
	if !b.BS || !b.HasCACH {
		return enums.SlotUnknown
	}
	c := layer2.DecodeCACHField(cach)
	if !m.ref || b.Stream != m.stream {
		m.t0, m.ref, m.stream, m.votes = b.Seconds, true, b.Stream, [2]int{}
	}
	parity := int(math.Round((b.Seconds-m.t0)/slotSeconds)) & 1
	if b.Stream != 0 {
		parity = int(b.Slot & 1)
	}
	tc := 0
	if c.TACT.TDMAChannel {
		tc = 1
	}
	m.votes[parity^tc]++
	align := 0
	if m.votes[1] > m.votes[0] {
		align = 1
	}
	return enums.Slot(parity ^ align + 1)
}
