package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

const slotSeconds = 0.03

type lcInfo struct {
	FLCO    string `json:"flco"`
	FID     uint8  `json:"fid"`
	Call    string `json:"call,omitempty"`
	Source  uint32 `json:"src,omitempty"`
	Dest    uint32 `json:"dst,omitempty"`
	Privacy bool   `json:"privacy,omitempty"`
	Emerg   bool   `json:"emergency,omitempty"`
	From    string `json:"from"`
	FEC     string `json:"fec"`
}

type smsInfo struct {
	Format string `json:"format"`
	Source uint32 `json:"src"`
	Dest   uint32 `json:"dst"`
	Group  bool   `json:"group,omitempty"`
	Text   string `json:"text"`
}

type gpsInfo struct {
	From      string  `json:"from"`
	Source    uint32  `json:"src,omitempty"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Detail    string  `json:"detail,omitempty"`
}

type event struct {
	Type      string   `json:"type"`
	T         float64  `json:"t"`
	Slot      int      `json:"slot"`
	Kind      string   `json:"kind"`
	Sync      string   `json:"sync,omitempty"`
	Distance  int      `json:"dist"`
	CC        int      `json:"cc"`
	DataType  string   `json:"dataType,omitempty"`
	Voice     string   `json:"voice,omitempty"`
	Fixed     int      `json:"fixedBits"`
	FEC       string   `json:"fec"`
	BER       float64  `json:"ber"`
	Frames    int      `json:"frames,omitempty"`
	AMBEBits  int      `json:"ambeBits,omitempty"`
	AMBEBER   float64  `json:"ambeBER,omitempty"`
	LC        *lcInfo  `json:"lc,omitempty"`
	CSBK      string   `json:"csbk,omitempty"`
	Header    string   `json:"header,omitempty"`
	Packet    string   `json:"packet,omitempty"`
	SMS       *smsInfo `json:"sms,omitempty"`
	GPS       *gpsInfo `json:"gps,omitempty"`
	Alias     string   `json:"alias,omitempty"`
	SLC       string   `json:"slc,omitempty"`
	Other     string   `json:"other,omitempty"`
	Error     string   `json:"error,omitempty"`
	Stream    int      `json:"stream"`
	DataBytes string   `json:"data"`
}

type slotStats struct {
	Bursts    int     `json:"bursts"`
	Voice     int     `json:"voiceFrames"`
	AMBEBits  int     `json:"ambeBits"`
	AMBEFixed int     `json:"ambeFixed"`
	AMBEBad   int     `json:"ambeBad"`
	BER       float64 `json:"ambeBER"`
}

type stats struct {
	Type      string                `json:"type"`
	Seconds   float64               `json:"seconds"`
	Bursts    int                   `json:"bursts"`
	Slots     map[string]*slotStats `json:"slots"`
	Offset    float64               `json:"offset"`
	Deviation float64               `json:"deviation"`
}

type chanState struct {
	voice layer2.VoiceReceiver
	asm   *packet.Assembler
	ta    pdu.TalkerAliasAssembler
	st    slotStats
}

type reporter struct {
	chans     [3]*chanState
	slc       layer2.ShortLCAssembler
	b         layer2.Burst
	loc       location.Decoder
	smsBuf    []byte
	t0        float64
	ref       bool
	votes     [2]int
	stream    int
	bursts    int
	out       []byte
	offset    float64
	deviation float64
}

func newReporter() *reporter {
	p := &reporter{smsBuf: make([]byte, 0, 4096), out: make([]byte, 0, 1<<16)}
	for i := range p.chans {
		p.chans[i] = &chanState{asm: packet.NewAssembler(127)}
	}
	return p
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func fecString(r fec.Result) string {
	switch {
	case r.Uncorrectable:
		return "bad"
	case r.ErrorsCorrected > 0:
		return fmt.Sprintf("fixed %d", r.ErrorsCorrected)
	}
	return "ok"
}

func lcOf(lc *pdu.FullLinkControl, from string) *lcInfo {
	l := &lcInfo{FLCO: lc.FLCO.String(), FID: uint8(lc.FID), From: from, FEC: fecString(lc.FEC)}
	if group, dst, src, ok := lc.Call(); ok {
		l.Call, l.Source, l.Dest = "private", src, dst
		if group {
			l.Call = "group"
		}
	}
	if so, ok := lc.ServiceOptions(); ok && l.Call != "" {
		l.Privacy, l.Emerg = so.IsPrivacy, so.IsEmergency
	}
	return l
}

func (p *reporter) emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	p.out = append(append(p.out, b...), '\n')
}

func (p *reporter) take() []byte {
	out := p.out
	p.out = p.out[len(p.out):]
	if cap(p.out) < 1<<12 {
		p.out = make([]byte, 0, 1<<16)
	}
	return out
}

func (p *reporter) slot(r *rf.Burst, cach [3]byte) int {
	if !r.BS || !r.HasCACH {
		return 0
	}
	c := layer2.DecodeCACHField(cach)
	if !p.ref || r.Stream != p.stream {
		p.t0, p.ref, p.stream, p.votes = r.Seconds, true, r.Stream, [2]int{}
	}
	parity := int(math.Round((r.Seconds-p.t0)/slotSeconds)) & 1
	if r.Stream != 0 {
		parity = int(r.Slot & 1)
	}
	p.votes[parity^b2i(c.TACT.TDMAChannel)]++
	align := 0
	if p.votes[1] > p.votes[0] {
		align = 1
	}
	if s, done, _ := p.slc.Add(c); done {
		p.emit(event{Type: "slc", T: r.Seconds, SLC: s.String(), FEC: fecString(s.FEC), Stream: r.Stream})
	}
	return parity ^ align + 1
}

func bitDiff(a, b layer2.RawBurst) int {
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
}

func (p *reporter) burst(r *rf.Burst) {
	p.bursts++
	data, cach := r.Decoded()
	ts := p.slot(r, cach)
	ch := p.chans[ts]
	ch.st.Bursts++
	e := event{Type: "burst", T: r.Seconds, Slot: ts, Distance: r.Distance, Fixed: bitDiff(data, r.Data), Stream: r.Stream, CC: -1,
		DataBytes: fmt.Sprintf("%x", data)}
	if !r.Timed() {
		e.Sync = r.Sync.String()
	}
	var opts layer2.DecodeOptions
	if r.Voice != enums.VoiceBurstUnknown {
		opts.Kind, opts.VoiceBurst = layer2.KindVoice, r.Voice
	} else if r.Distance > layer2.DefaultSyncThreshold {
		opts.SyncThreshold = r.Distance
	}
	b := &p.b
	if err := b.DecodeFromBytesWith(data, &opts); err != nil {
		e.Kind, e.Error = "undecodable", err.Error()
		p.emit(e)
		return
	}
	lc, lcDone, lcErr := ch.voice.Receive(b, nil)
	if r.Voice != enums.VoiceBurstUnknown {
		b.VoiceBurst = r.Voice
	}
	e.Kind = b.Kind.String()
	if b.HasEmbeddedSignalling() {
		e.CC = int(b.EmbeddedSignalling.ColorCode)
	}
	if b.Kind == layer2.KindVoice {
		e.Voice = b.VoiceBurst.String()
		_, res := b.AMBEFrames()
		ch.st.Voice += layer2.VoiceSlotsPerBurst
		ch.st.AMBEBits += res.BitsChecked
		ch.st.AMBEFixed += res.ErrorsCorrected
		ch.st.AMBEBad += b2i(res.Uncorrectable)
		if ch.st.AMBEBits > 0 {
			ch.st.BER = float64(ch.st.AMBEFixed) / float64(ch.st.AMBEBits)
		}
		e.Frames, e.AMBEBits, e.AMBEBER = ch.st.Voice, ch.st.AMBEBits, ch.st.BER
		e.FEC = fecString(res)
		if res.BitsChecked > 0 {
			e.BER = float64(res.ErrorsCorrected) / float64(res.BitsChecked)
		}
	} else {
		e.CC = int(b.SlotType.ColorCode)
		e.DataType = b.SlotType.DataType.String()
		agg := b.FEC.Aggregate()
		e.FEC = fecString(agg)
		if agg.BitsChecked > 0 {
			e.BER = float64(agg.ErrorsCorrected) / float64(agg.BitsChecked)
		}
	}
	switch {
	case lcDone:
		e.LC = lcOf(&lc, "embedded")
		p.embedded(&e, ch, &lc)
	case errors.Is(lcErr, layer2.ErrEmbeddedLCUncorrectable):
		e.Other = "embedded LC failed its checksum"
	}
	switch d := b.Data.(type) {
	case nil:
	case *pdu.FullLinkControl:
		from := "header"
		if d.BurstDataType == enums.DataTypeTerminatorWithLC {
			from = "terminator"
		}
		e.LC = lcOf(d, from)
	case *pdu.CSBK:
		e.CSBK = d.String()
	case *pdu.DataHeader:
		e.Header = d.String()
	default:
		if b.SlotType.DataType != enums.DataTypeIdle {
			e.Other = d.String()
		}
	}
	p.packet(&e, ch, b)
	p.emit(e)
}

func (p *reporter) embedded(e *event, ch *chanState, lc *pdu.FullLinkControl) {
	if g, ok := lc.AsGPSInfo(); ok {
		e.GPS = &gpsInfo{From: "embedded LC", Latitude: g.Latitude, Longitude: g.Longitude, Detail: g.PositionError.String()}
	}
	if alias, ok, err := ch.ta.Add(lc); ok && err == nil {
		e.Alias = alias
	}
}

func (p *reporter) packet(e *event, ch *chanState, b *layer2.Burst) {
	m, ok, err := ch.asm.AddBurst(b)
	switch {
	case err != nil:
		e.Error = "packet: " + err.Error()
		return
	case !ok:
		return
	}
	dst, src, _ := m.Header.Addresses()
	e.Packet = fmt.Sprintf("%s %d to %d, %d octets, %s, CRC %s", m.Header.Format(), src, dst, len(m.Data), m.DataType, map[bool]string{true: "bad", false: "ok"}[m.BadCRC])
	if h, ok := m.Header.AsResponse(); ok {
		if r, err := packet.ResponseOf(&h); err == nil {
			e.Packet += ", response " + r.String()
		}
		return
	}
	if m.BadCRC {
		return
	}
	var t sms.Text
	if p.smsBuf, t, err = sms.Decode(p.smsBuf[:0], &m.Header, m.Data); err == nil {
		e.SMS = &smsInfo{Format: t.Format.String(), Source: t.Source, Dest: t.Destination, Group: t.Group, Text: string(t.Body)}
		return
	}
	smsErr := err
	fix, err := p.loc.Decode(&m.Header, m.Data)
	if err != nil {
		if !errors.Is(smsErr, sms.ErrNotText) {
			e.Error = "SMS: " + smsErr.Error()
		}
		return
	}
	{
		g := &gpsInfo{From: fix.Format.String(), Source: fix.Source, Latitude: fix.Latitude, Longitude: fix.Longitude}
		if fix.HasRadius {
			g.Detail = fmt.Sprintf("radius %.0f m", fix.Radius)
		}
		if !fix.Valid {
			g.Detail = "no GNSS fix"
		}
		e.GPS = g
	}
}

func (p *reporter) summary(seconds float64) stats {
	s := stats{Type: "stats", Seconds: seconds, Bursts: p.bursts, Slots: map[string]*slotStats{}, Offset: p.offset, Deviation: p.deviation}
	for i, ch := range p.chans {
		if ch.st.Bursts == 0 {
			continue
		}
		name := fmt.Sprint(i)
		if i == 0 {
			name = "-"
		}
		st := ch.st
		s.Slots[name] = &st
	}
	return s
}
