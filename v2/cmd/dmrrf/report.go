package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"strings"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func fecString(r fec.Result) string {
	switch {
	case r.Uncorrectable:
		return "bad"
	case r.ErrorsCorrected > 0:
		return fmt.Sprintf("fixed:%d", r.ErrorsCorrected)
	}
	return "ok"
}

func b01(v bool) int {
	if v {
		return 1
	}
	return 0
}

func lcFields(lc *pdu.FullLinkControl) string {
	s := fmt.Sprintf("flco=%d fid=0x%02x", lc.FLCO, uint8(lc.FID))
	switch {
	case lc.GroupVoice != nil:
		s += " group"
	case lc.UnitToUnit != nil:
		s += " private"
	}
	if dst, src, ok := lc.Addresses(); ok {
		s += fmt.Sprintf(" src=%d dst=%d", src, dst)
	}
	if so, ok := lc.ServiceOptions(); ok && (lc.GroupVoice != nil || lc.UnitToUnit != nil) {
		s += fmt.Sprintf(" privacy=%d emergency=%d", b01(so.IsPrivacy), b01(so.IsEmergency))
	}
	return s
}

// chanState is the receive state of one TDMA channel.
type chanState struct {
	voice layer2.VoiceReceiver
	asm   *packet.Assembler
	ambe  int
}

// Reporter prints the bursts of a capture, one item per line with a fixed
// first word: cach and slc for the CACH of BS-sourced bursts, emb, ambe and
// elc for voice, lc for the voice LC header and terminator, pi for the PI
// header, and packet and response for data.
//
// The TDMA channel of a BS-sourced burst, ts=1 or ts=2, comes from its slot
// number in its stream, or its slot time when the burst has no stream,
// which alternates every 30 ms, aligned with the TC bits of the CACHs of
// the stream by majority. The Hamming(7,4,3) code of the TACT corrects every error
// pattern to some codeword, so a single TC bit cannot be trusted.
type Reporter struct {
	w      io.Writer
	chans  map[int]*chanState
	slc    layer2.ShortLCAssembler
	t0     float64
	ref    bool
	votes  [2]int
	stream int
}

// NewReporter returns a Reporter that writes to w.
func NewReporter(w io.Writer) *Reporter {
	return &Reporter{w: w, chans: map[int]*chanState{}}
}

func (p *Reporter) printf(format string, a ...any) {
	fmt.Fprintf(p.w, format+"\n", a...)
}

// Burst reports one received burst.
func (p *Reporter) Burst(r Received) {
	t := fmt.Sprintf("t=%.2fms", r.Seconds*1000)
	data, cach := r.decoded()
	fixed := ""
	if n := bitDiff(data, r.Data); n > 0 {
		fixed = fmt.Sprintf("  fixed %d", n)
	}
	if r.Timed() {
		p.printf("%9.2f ms  no sync (timed)  %x%s", r.Seconds*1000, data, fixed)
	} else {
		p.printf("%9.2f ms  sync distance %d  %x%s", r.Seconds*1000, r.Distance, data, fixed)
	}
	ts := "-"
	key := 0
	if r.BS && r.HasCACH {
		c := layer2.DecodeCACHField(cach)
		if !p.ref || r.Stream != p.stream {
			p.t0, p.ref, p.stream, p.votes = r.Seconds, true, r.Stream, [2]int{}
		}
		parity := int(math.Round((r.Seconds-p.t0)/slotSeconds)) & 1
		if r.Stream != 0 {
			parity = int(r.Slot & 1)
		}
		p.votes[parity^b01(c.TACT.TDMAChannel)]++
		align := 0
		if p.votes[1] > p.votes[0] {
			align = 1
		}
		key = parity ^ align + 1
		ts = fmt.Sprint(key)
		p.printf("cach %s at=%d tc=%d lcss=%q fec=%s payload=%s", t, b01(c.TACT.AccessType), b01(c.TACT.TDMAChannel),
			c.TACT.LCSS.String(), fecString(c.TACT.FEC), bitString(c.Payload[:]))
		if s, done, _ := p.slc.Add(c); done {
			p.printf("slc %s %s fec=%s", t, slcFields(&s), fecString(s.FEC))
		}
	}
	t += " ts=" + ts
	ch := p.chans[key]
	if ch == nil {
		ch = &chanState{asm: packet.NewAssembler(127)}
		p.chans[key] = ch
	}

	var opts layer2.DecodeOptions
	if r.Voice != enums.VoiceBurstUnknown {
		opts.Kind, opts.VoiceBurst = layer2.KindVoice, r.Voice
	} else if r.Distance > layer2.DefaultSyncThreshold {
		opts.SyncThreshold = r.Distance
	}
	b := &layer2.Burst{}
	if err := b.DecodeFromBytesWith(data, &opts); err != nil {
		p.printf("           decode error: %v", err)
		return
	}
	lc, lcDone, lcErr := ch.voice.Receive(b)
	if r.Voice != enums.VoiceBurstUnknown {
		b.VoiceBurst = r.Voice
	}
	p.printf("           %s", b.String())
	if b.HasEmbeddedSignalling() {
		e := b.EmbeddedSignalling
		p.printf("emb %s pos=%s cc=%d pi=%d lcss=%q fec=%s", t, b.VoiceBurst, e.ColorCode, b01(e.PreemptionAndPowerControlIndicator), e.LCSS.String(), fecString(e.FEC))
	}
	if b.Kind == layer2.KindVoice {
		for i, slot := range b.VoiceSlots {
			f, res := vocoder.DecodeAMBE72(slot)
			p.printf("ambe %s pos=%s seq=%d n=%d 0x%013x fec=%s", t, b.VoiceBurst, ch.ambe, i, ambeValue(f), fecString(res))
			ch.ambe++
		}
	}
	switch {
	case lcDone:
		p.printf("elc %s %s checksum=ok fec=%s", t, lcFields(&lc), fecString(lc.FEC))
	case errors.Is(lcErr, layer2.ErrEmbeddedLCUncorrectable):
		p.printf("elc %s %s checksum=bad", t, lcFields(&lc))
	case lcErr != nil:
		p.printf("elc %s %v", t, lcErr)
	}
	switch d := b.Data.(type) {
	case *pdu.FullLinkControl:
		kind := "header"
		if d.BurstDataType == enums.DataTypeTerminatorWithLC {
			kind = "terminator"
		} else {
			ch.ambe = 0
		}
		p.printf("lc %s %s %s fec=%s", t, kind, lcFields(d), fecString(d.FEC))
	case *pdu.PIHeader:
		p.printf("pi %s algid=0x%02x mfid=0x%02x keyid=0x%02x mi=0x%08x dst=%d fec=%s", t, d.AlgorithmID, uint8(d.MFID), d.KeyID, d.MessageIndicator, d.Destination, fecString(d.FEC))
	}
	m, ok, err := ch.asm.AddBurst(b)
	switch {
	case err != nil:
		p.printf("           packet: %v", err)
	case ok && m.Header.ResponsePacketHeader != nil:
		h := m.Header.ResponsePacketHeader
		line := fmt.Sprintf("response %s %s class=%d type=%d status=%d %d -> %d", t, strings.ReplaceAll(packet.ResponseOf(h).String(), " ", "_"),
			h.ResponseClass, h.ResponseType, h.ResponseStatus, h.LLIDSource, h.LLIDDestination)
		if m.Response != nil {
			line += " retry=" + intList(m.Response.RetryBlocks())
		}
		p.printf("%s", line)
	case ok:
		p.printf("           packet complete: %q (%s)", m.Data, m.DataType)
	}
}

func bitString(b []bit.Bit) string {
	var s strings.Builder
	for _, v := range b {
		s.WriteByte('0' + byte(v))
	}
	return s.String()
}

func slcFields(s *pdu.ShortLC) string {
	if a := s.ActivityUpdate; a != nil {
		return fmt.Sprintf("slco=%d activity ts1=0x%x hash1=0x%02x ts2=0x%x hash2=0x%02x", s.SLCO, uint8(a.TS1ActivityID), a.HashTS1, uint8(a.TS2ActivityID), a.HashTS2)
	}
	return fmt.Sprintf("slco=%d %s", s.SLCO, s.String())
}

// ambeValue returns the 49 bits of an AMBE+2 frame as a number, the form
// that tx and rx print.
func ambeValue(f vocoder.AMBEFrame) uint64 { return bit.Uint(f[:]) }

func intList(v []int) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ",")
}

// bitDiff returns the number of bits in which a and b differ.
func bitDiff(a, b layer2.RawBurst) int {
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
}
