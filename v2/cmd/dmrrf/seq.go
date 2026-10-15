package main

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// SeqConfig describes a transmission.
type SeqConfig struct {
	ColorCode uint8
	Source    uint32
	Dest      uint32
	BS        uint32
	Group     bool
	// Wakeups is the number of wakeup CSBKs before an MS-sourced sequence,
	// or of idle frames before a BS-sourced one.
	Wakeups int
	Message string
	// Superframes is the number of voice superframes of a voice call.
	Superframes int
	// AMBESilence sends AMBE+2 silence instead of the test pattern.
	AMBESilence bool
	// LateEntry leaves out the voice LC and PI headers, so a receiver learns
	// the call only from the embedded LC.
	LateEntry bool
	// Preambles is the number of preamble CSBKs of the preamble sequence.
	Preambles int
	// BSSourced builds a continuous BS outbound stream with the sequence on
	// Slot and idle bursts on the other slot.
	BSSourced bool
	Slot      int
}

// Built is a transmission: Bursts for an MS-sourced one, Slots for a
// BS-sourced stream, and Info lines that describe the content.
type Built struct {
	Bursts [][33]byte
	Slots  []BSSlot
	Info   []string
}

// Names of the sequences that are not data packets.
const (
	seqWakeup   = "wakeup"
	seqACK      = "ack"
	seqSR       = "sr"
	seqPreamble = "preamble"
	seqVoice    = "voice"
	seqPI       = "pi"
	seqRC       = "rc"
	seqMBC      = "mbc"
	seqTier3    = "tier3"
)

// Sequences returns the names Sequence accepts.
func Sequences() []string {
	return []string{
		seqWakeup, "data12", "data34", "data1", "data12u", "data34u", "data1u",
		seqACK, seqSR, seqPreamble, seqVoice, seqPI, seqRC, seqMBC, seqTier3,
	}
}

// PI header values of the pi sequence (DMRA layout, Motorola MFID).
const (
	piAlgorithmID = 0x21
	piKeyID       = 0x01
	piMI          = 0x12345678
	piMFID        = 0x10
)

// srBlocks is the size of the packet the sr sequence answers, and
// srRetry the DBSNs it asks to be sent again.
const srBlocks = 8

func srRetry() []int { return []int{1, 4, 6} }

func putAddr(b []byte, a uint32) {
	var t [4]byte
	binary.BigEndian.PutUint32(t[:], a)
	copy(b, t[1:])
}

// WakeupBurst returns an MS-sourced BS_Dwn_Act CSBK burst (ETSI TS 102 361-2
// clause 7.1.2.1, Table 7.5a) that activates a BS outbound.
func WakeupBurst(cfg SeqConfig) ([33]byte, error) {
	var c pdu.CSBKBytes
	c[0] = 0x80 | 0x38
	putAddr(c[4:7], cfg.BS)
	putAddr(c[7:10], cfg.Source)
	c.Reseal()
	r, err := layer2.BuildDataBurst([12]byte(c), enums.DataTypeCSBK, cfg.ColorCode, layer2.SyncSourceMS)
	return [33]byte(r), err
}

func rateOf(name string) (enums.DataType, bool, error) {
	confirmed := !strings.HasSuffix(name, "u")
	switch strings.TrimSuffix(name, "u") {
	case "data12":
		return enums.DataTypeRate12, confirmed, nil
	case "data34":
		return enums.DataTypeRate34, confirmed, nil
	case "data1":
		return enums.DataTypeRate1, confirmed, nil
	}
	return 0, false, fmt.Errorf("unknown data sequence %q", name)
}

// DataPacket segments the test message into a packet at the rate the
// sequence name selects: confirmed for data12, data34 and data1, and
// unconfirmed for data12u, data34u and data1u.
func DataPacket(name string, cfg SeqConfig) (packet.Packet, error) {
	rate, confirmed, err := rateOf(name)
	if err != nil {
		return packet.Packet{}, err
	}
	h := pdu.DataHeader{
		Format: pdu.FormatConfirmed,
		ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			Group:             cfg.Group,
			ResponseRequested: !cfg.Group,
			SAP:               enums.SAPShortData,
			LLIDDestination:   cfg.Dest,
			LLIDSource:        cfg.Source,
			FullMessageFlag:   true,
		},
	}
	if !confirmed {
		h = pdu.DataHeader{
			Format: pdu.FormatUnconfirmed,
			UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
				Group:           cfg.Group,
				SAP:             enums.SAPShortData,
				LLIDDestination: cfg.Dest,
				LLIDSource:      cfg.Source,
				FullMessageFlag: true,
			},
		}
	}
	return packet.Segment(h, []byte(cfg.Message), rate)
}

func (cfg SeqConfig) source() layer2.SyncSource {
	if cfg.BSSourced {
		return layer2.SyncSourceBS
	}
	return layer2.SyncSourceMS
}

func dataBursts(name string, cfg SeqConfig) ([][33]byte, error) {
	p, err := DataPacket(name, cfg)
	if err != nil {
		return nil, err
	}
	return packet.Bursts(p, cfg.ColorCode, cfg.source())
}

// responseBursts builds the response that -src sends to -dst for a
// confirmed rate 1/2 packet of srBlocks blocks with N(S) 0: an ACK, or for
// sack a selective retry of the srRetry blocks.
func responseBursts(sack bool, cfg SeqConfig) ([][33]byte, []string, error) {
	to := &pdu.ConfirmedDataHeader{
		SAP:             enums.SAPShortData,
		LLIDDestination: cfg.Source,
		LLIDSource:      cfg.Dest,
		FullMessageFlag: true,
		BlocksToFollow:  srBlocks,
	}
	r := packet.ResponseACK
	var data *packet.ResponseData
	var info []string
	if sack {
		r = packet.ResponseSACK
		data = &packet.ResponseData{}
		for _, i := range srRetry() {
			data.Retry[i] = true
		}
		blocks, err := data.Encode(srBlocks)
		if err != nil {
			return nil, nil, err
		}
		info = append(info, fmt.Sprintf("sr retry=%s of %d blocks, C_RDATA %x", intList(srRetry()), srBlocks, blocks[0].Data))
	}
	p, err := packet.NewResponse(to, r, data)
	if err != nil {
		return nil, nil, err
	}
	h := p.Header.ResponsePacketHeader
	info = append(info, fmt.Sprintf("response %s class=%d type=%d status=%d %d -> %d", r, h.ResponseClass, h.ResponseType, h.ResponseStatus, h.LLIDSource, h.LLIDDestination))
	b, err := packet.Bursts(p, cfg.ColorCode, cfg.source())
	return b, info, err
}

func csbkBurst(c *pdu.CSBK, cfg SeqConfig) ([33]byte, error) {
	bits, err := c.Encode()
	if err != nil {
		return [33]byte{}, err
	}
	r, err := layer2.BuildDataBurst(bit.Pack96(bits), enums.DataTypeCSBK, cfg.ColorCode, cfg.source())
	return [33]byte(r), err
}

// preambleBursts returns Preambles Pre_CSBKs (ETSI TS 102 361-2 clause
// 7.1.2.5) followed by a confirmed rate 1/2 packet. The CSBK Blocks to
// Follow counts the preambles still to come and the bursts of the packet.
func preambleBursts(cfg SeqConfig) ([][33]byte, error) {
	data, err := dataBursts("data12", cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Preambles-1+len(data) > 0xFF {
		return nil, fmt.Errorf("%d preambles and %d data bursts do not fit in CSBK Blocks to Follow", cfg.Preambles, len(data))
	}
	var out [][33]byte
	for i := range cfg.Preambles {
		c := &pdu.CSBK{
			LastBlock: true,
			Opcode:    pdu.CSBKPreamble,
			PreamblePDU: &pdu.PreamblePDU{
				Data:               true,
				Group:              cfg.Group,
				CSBKBlocksToFollow: byte(cfg.Preambles - 1 - i + len(data)), //nolint:gosec // checked above
				TargetAddress:      cfg.Dest,
				SourceAddress:      cfg.Source,
			},
		}
		b, err := csbkBurst(c, cfg)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return append(out, data...), nil
}

// TestAMBEFrame returns frame i of the voice test pattern: a distinct,
// known 49-bit value per frame, so the relayed frames can be compared bit
// for bit.
func TestAMBEFrame(i int) vocoder.AMBEFrame {
	z := uint64(i+1) * 0x9E3779B97F4A7C15 //nolint:gosec // i is small and positive
	z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
	z = (z ^ z>>27) * 0x94D049BB133111EB
	z ^= z >> 31
	var f vocoder.AMBEFrame
	bit.PutUint(f[:], z&(1<<49-1))
	return f
}

func callLC(cfg SeqConfig, privacy bool) pdu.FullLinkControl {
	so := layer3.ServiceOptions{IsPrivacy: privacy}
	if cfg.Group {
		return pdu.FullLinkControl{
			FLCO:       enums.FLCOGroupVoiceChannelUser,
			GroupVoice: &pdu.FLCGroupVoice{ServiceOptions: so, GroupAddress: cfg.Dest, SourceAddress: cfg.Source},
		}
	}
	return pdu.FullLinkControl{
		FLCO:       enums.FLCOUnitToUnitVoiceChannelUser,
		UnitToUnit: &pdu.FLCUnitToUnit{ServiceOptions: so, TargetAddress: cfg.Dest, SourceAddress: cfg.Source},
	}
}

// voiceBursts builds a voice call: the voice LC header, the PI header for
// privacy, Superframes superframes of the AMBE test pattern and the
// terminator with LC.
func voiceBursts(cfg SeqConfig, privacy bool) ([][33]byte, []string, error) {
	opts := layer2.VoiceTransmitterOptions{
		LC:        callLC(cfg, privacy),
		ColorCode: cfg.ColorCode,
		Source:    cfg.source(),
		Silence:   vocoder.EncodeAMBE72(vocoder.AMBESilence()),
	}
	var info []string
	if privacy {
		opts.PIHeader = &pdu.PIHeader{
			AlgorithmID:      piAlgorithmID,
			MFID:             piMFID,
			KeyID:            piKeyID,
			MessageIndicator: piMI,
			Destination:      cfg.Dest,
		}
		info = append(info, fmt.Sprintf("pi algid=0x%02x mfid=0x%02x keyid=0x%02x mi=0x%08x dst=%d", piAlgorithmID, piMFID, piKeyID, piMI, cfg.Dest))
	}
	t, err := layer2.NewVoiceTransmitterWithOptions(opts)
	if err != nil {
		return nil, nil, err
	}
	out := t.Headers()
	if cfg.LateEntry {
		out = nil
	}
	n := cfg.Superframes * layer2.VoiceBurstsPerSuperframe * layer2.VoiceSlotsPerBurst
	for i := range n {
		f := TestAMBEFrame(i)
		if cfg.AMBESilence {
			f = vocoder.AMBESilence()
		}
		info = append(info, fmt.Sprintf("ambe tx seq=%d 0x%013x", i, ambeValue(f)))
		b, err := t.AddFrames(f)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, b...)
	}
	b, err := t.Terminate()
	if err != nil {
		return nil, nil, err
	}
	return append(out, b...), info, nil
}

// rcBurst returns a standalone MS-sourced reverse channel burst (ETSI TS 102
// 361-1 clause 6.4.1) with the cease transmission command.
func rcBurst(cfg SeqConfig) ([33]byte, error) {
	b := layer2.Burst{
		Kind:               layer2.KindReverseChannel,
		SyncPattern:        enums.SyncMSSourcedRC,
		EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: cfg.ColorCode},
		ReverseChannel:     pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand},
	}
	return b.Encode()
}

// Tier III values of the tier3 and mbc sequences.
const (
	tier3SysCode = 0x1234
	tier3Channel = 1
	tier3MHz     = 446
	tier3Steps   = 50
)

// mbcBursts returns a TV_GRANT with absolute channel parameters (ETSI TS 102
// 361-4 clause 7.1.1.1.1): an MBC header whose physical channel is 0xFFF and
// a CG_AP continuation with the channel frequencies.
func mbcBursts(cfg SeqConfig) ([][33]byte, []string, error) {
	g := pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF, TargetAddress: cfg.Dest, SourceAddress: cfg.Source}
	gb, err := g.Encode()
	if err != nil {
		return nil, nil, err
	}
	h := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, Data: [8]byte(bit.Pack(gb[:]))}
	hb, err := h.Encode()
	if err != nil {
		return nil, nil, err
	}
	c := pdu.CGAPContinuation{
		LastBlock: true,
		Opcode:    pdu.CSBKTalkgroupVoiceGrant,
		ColorCode: cfg.ColorCode,
		CdefParms: layer3.CdefParms{Channel: tier3Channel, TXMHz: tier3MHz, TXKHz: tier3Steps, RXMHz: tier3MHz, RXKHz: tier3Steps},
	}
	cb, err := c.Encode()
	if err != nil {
		return nil, nil, err
	}
	r1, err := layer2.BuildDataBurst(bit.Pack96(hb), enums.DataTypeMBCHeader, cfg.ColorCode, cfg.source())
	if err != nil {
		return nil, nil, err
	}
	r2, err := layer2.BuildDataBurst(bit.Pack96(cb), enums.DataTypeMBCContinuation, cfg.ColorCode, cfg.source())
	if err != nil {
		return nil, nil, err
	}
	info := []string{fmt.Sprintf("mbc TV_GRANT %d -> %d channel 0xFFF, CG_AP channel %d tx/rx %d MHz + %d x 125 Hz", cfg.Source, cfg.Dest, tier3Channel, tier3MHz, tier3Steps)}
	return [][33]byte{[33]byte(r1), [33]byte(r2)}, info, nil
}

// tier3Bursts returns C_ALOHA, TV_GRANT and C_MOVE CSBKs and the MBC grant.
func tier3Bursts(cfg SeqConfig) ([][33]byte, []string, error) {
	csbks := []*pdu.CSBK{
		{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{SiteTSSync: true, NRandWait: 4, Backoff: 1, SysIdentCode: tier3SysCode, MSAddress: cfg.Dest}},
		{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: tier3Channel, TargetAddress: cfg.Dest, SourceAddress: cfg.Source}},
		{LastBlock: true, Opcode: pdu.CSBKMove, MovePDU: &pdu.MovePDU{PhysicalChannel: tier3Channel, MSAddress: cfg.Dest}},
	}
	var out [][33]byte
	for _, c := range csbks {
		b, err := csbkBurst(c, cfg)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, b)
	}
	mbc, info, err := mbcBursts(cfg)
	if err != nil {
		return nil, nil, err
	}
	info = append([]string{fmt.Sprintf("tier3 C_ALOHA syscode=0x%04x ms=%d, TV_GRANT %d -> %d channel %d, C_MOVE ms=%d channel %d",
		tier3SysCode, cfg.Dest, cfg.Source, cfg.Dest, tier3Channel, cfg.Dest, tier3Channel)}, info...)
	return append(out, mbc...), info, nil
}

func payload(name string, cfg SeqConfig) ([][33]byte, []string, error) {
	switch name {
	case seqWakeup:
		return nil, nil, nil
	case seqACK, seqSR:
		return responseBursts(name == seqSR, cfg)
	case seqPreamble:
		if cfg.Preambles < 0 || cfg.Preambles > 200 {
			return nil, nil, fmt.Errorf("%d preambles, want 0 to 200", cfg.Preambles)
		}
		b, err := preambleBursts(cfg)
		return b, nil, err
	case seqVoice, seqPI:
		if cfg.Slot < 0 {
			return nil, nil, fmt.Errorf("voice needs -slot 0 or 1: bursts B to F are found 60 ms after burst A")
		}
		return voiceBursts(cfg, name == seqPI)
	case seqRC:
		if cfg.BSSourced {
			return nil, nil, fmt.Errorf("a standalone RC burst is MS-sourced")
		}
		b, err := rcBurst(cfg)
		return [][33]byte{b}, []string{"rc command=" + enums.RCCeaseTransmissionCommand.String()}, err
	case seqMBC, seqTier3:
		if !cfg.BSSourced {
			return nil, nil, fmt.Errorf("%s sends outbound Tier III PDUs and needs -bs-sourced", name)
		}
		if name == seqMBC {
			return mbcBursts(cfg)
		}
		return tier3Bursts(cfg)
	}
	b, err := dataBursts(name, cfg)
	return b, nil, err
}

// Sequence builds the bursts of a named transmission (see Sequences). An
// MS-sourced one starts with Wakeups BS_Dwn_Act CSBKs; "wakeup" sends only
// those. A BS-sourced one is a continuous stream (see BSStream).
func Sequence(name string, cfg SeqConfig) (Built, error) {
	p, info, err := payload(name, cfg)
	if err != nil {
		return Built{}, err
	}
	if cfg.BSSourced {
		if name == seqWakeup {
			return Built{}, fmt.Errorf("wakeup is MS-sourced")
		}
		slots, err := BSStream(p, cfg, activityOf(name, cfg.Group))
		return Built{Bursts: p, Slots: slots, Info: info}, err
	}
	wake, err := WakeupBurst(cfg)
	if err != nil {
		return Built{}, err
	}
	out := make([][33]byte, 0, cfg.Wakeups+len(p))
	for range cfg.Wakeups {
		out = append(out, wake)
	}
	return Built{Bursts: append(out, p...), Info: info}, nil
}

func activityOf(name string, group bool) enums.ActivityID {
	switch {
	case name == seqVoice || name == seqPI:
		if group {
			return enums.ActivityGroupVoice
		}
		return enums.ActivityIndividualVoice
	case name == seqTier3 || name == seqMBC:
		if group {
			return enums.ActivityGroupCSBK
		}
		return enums.ActivityIndividualCSBK
	case group && (strings.HasPrefix(name, "data") || name == seqPreamble):
		return enums.ActivityGroupData
	}
	return enums.ActivityIndividualData
}

// IdleBurst returns a BS-sourced idle burst (ETSI TS 102 361-1 clause
// 7.3.2).
func IdleBurst(cc uint8) ([33]byte, error) {
	r, err := layer2.BuildDataBurst(bit.Pack96(layer2.IdleMessageInfo()), enums.DataTypeIdle, cc, layer2.SyncSourceBS)
	return [33]byte(r), err
}

// BSStream lays bursts out as a BS outbound stream: Wakeups idle frames,
// the bursts on cfg.Slot with idle bursts on the other slot, then two idle
// frames, padded to an even number of frames so the stream is a whole
// number of Short LCs. Every CACH carries a TACT with the TDMA channel of
// the burst that follows and AT set while that channel is active, and one
// row of an Activity Update Short LC, which spans four CACHs. The Short LC
// announces the activity on cfg.Slot while any of its four slots carries
// the sequence, and no activity otherwise.
func BSStream(bursts [][33]byte, cfg SeqConfig, activity enums.ActivityID) ([]BSSlot, error) {
	if cfg.Slot != 0 && cfg.Slot != 1 {
		return nil, fmt.Errorf("a BS-sourced stream needs -slot 0 or 1, not %d", cfg.Slot)
	}
	idle, err := IdleBurst(cfg.ColorCode)
	if err != nil {
		return nil, err
	}
	lead := max(cfg.Wakeups, 0)
	busy := func(f int) bool { return f >= lead && f-lead < len(bursts) }
	frames := lead + len(bursts) + 2
	frames += frames % 2
	out := make([]BSSlot, 0, 2*frames)
	for f := range frames {
		au := &pdu.ShortLCActivityUpdate{}
		if busy(f-f%2) || busy(f-f%2+1) {
			hash := pdu.AddressHash(cfg.Dest)
			if cfg.Slot == 0 {
				au.TS1ActivityID, au.HashTS1 = activity, hash
			} else {
				au.TS2ActivityID, au.HashTS2 = activity, hash
			}
		}
		slc := pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: au}
		for ts := range 2 {
			s := BSSlot{Burst: idle}
			active := ts == cfg.Slot && busy(f)
			if active {
				s.Burst = bursts[f-lead]
			}
			cach, err := layer2.EncodeShortLCToCACH(&slc, pdu.TACT{AccessType: active, TDMAChannel: ts == 1})
			if err != nil {
				return nil, err
			}
			s.CACH = cach[len(out)%4]
			out = append(out, s)
		}
	}
	return out, nil
}
