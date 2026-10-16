package rftx

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// SeqConfig describes a transmission.
type SeqConfig struct {
	ColorCode enums.ColorCode
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
// BS-sourced stream, Carriers for the other channels of a multi-channel
// one, and Info lines that describe the content.
type Built struct {
	Bursts   []layer2.RawBurst
	Slots    []BSSlot
	Carriers []SiteCarrier
	Info     []string
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
	seqSite     = "t3site"
	seqRadioChk = "radiocheck"
	seqAlert    = "callalert"
)

// Sequences returns the names Sequence accepts.
func Sequences() []string {
	return []string{
		seqWakeup, "data12", "data34", "data1", "data12u", "data34u", "data1u",
		seqACK, seqSR, seqPreamble, seqVoice, seqPI, seqRC, seqMBC, seqTier3, seqSite, seqRadioChk, seqAlert,
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

// WakeupBurst returns an MS-sourced BS_Dwn_Act CSBK burst (ETSI TS 102 361-2
// clause 7.1.2.1, Table 7.5a) that activates a BS outbound.
func WakeupBurst(cfg SeqConfig) (layer2.RawBurst, error) {
	c, err := pdu.NewBSOutboundActivation(cfg.BS, cfg.Source)
	if err != nil {
		return layer2.RawBurst{}, err
	}
	return layer2.BuildBurst(&c, cfg.ColorCode, layer2.SyncSourceMS)
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
	return packet.NewPacket(h, []byte(cfg.Message), rate)
}

func (cfg SeqConfig) source() layer2.SyncSource {
	if cfg.BSSourced {
		return layer2.SyncSourceBS
	}
	return layer2.SyncSourceMS
}

// DataBursts returns the bursts of the data packet of sequence name.
func DataBursts(name string, cfg SeqConfig) ([]layer2.RawBurst, error) {
	p, err := DataPacket(name, cfg)
	if err != nil {
		return nil, err
	}
	return packet.Bursts(p, cfg.ColorCode, cfg.source())
}

// responseBursts builds the response that -src sends to -dst for a
// confirmed rate 1/2 packet of srBlocks blocks with N(S) 0: an ACK, or for
// sack a selective retry of the srRetry blocks.
func responseBursts(sack bool, cfg SeqConfig) ([]layer2.RawBurst, []string, error) {
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

// preambleBursts returns Preambles Pre_CSBKs (ETSI TS 102 361-2 clause
// 7.1.2.5) followed by a confirmed rate 1/2 packet. The CSBK Blocks to
// Follow counts the preambles still to come and the bursts of the packet.
func preambleBursts(cfg SeqConfig) ([]layer2.RawBurst, error) {
	data, err := DataBursts("data12", cfg)
	if err != nil {
		return nil, err
	}
	toFollow := cfg.Preambles - 1 + len(data)
	if toFollow < 0 || toFollow > 0xFF {
		return nil, fmt.Errorf("%d preambles and %d data bursts do not fit in CSBK Blocks to Follow", cfg.Preambles, len(data))
	}
	blocks := byte(toFollow)
	var out []layer2.RawBurst
	for range cfg.Preambles {
		c := &pdu.CSBK{
			LastBlock: true,
			Opcode:    pdu.CSBKPreamble,
			PreamblePDU: &pdu.PreamblePDU{
				Data:               true,
				Group:              cfg.Group,
				CSBKBlocksToFollow: blocks,
				TargetAddress:      cfg.Dest,
				SourceAddress:      cfg.Source,
			},
		}
		b, err := layer2.BuildBurst(c, cfg.ColorCode, cfg.source())
		if err != nil {
			return nil, err
		}
		out = append(out, b)
		blocks--
	}
	return append(out, data...), nil
}

// Motorola FID 0x10 CSBK opcodes, as an AnyTone sends them through an XPR
// 8400 (capture runs/moto-rc-1).
const (
	motoFID        = 0x10
	motoRadioCheck = 0x24
	motoCallAlert  = 0x1F
)

// motorolaCSBKBursts returns Preambles Pre_CSBKs and a FID 0x10 radio check
// or call alert. The radio check carries the source before the target, the
// call alert the target before the source.
func motorolaCSBKBursts(alert bool, cfg SeqConfig) ([]layer2.RawBurst, error) {
	if cfg.Preambles < 0 || cfg.Preambles > 0xFF {
		return nil, fmt.Errorf("%d preambles, want 0 to 255", cfg.Preambles)
	}
	var out []layer2.RawBurst
	for i := range cfg.Preambles {
		c := &pdu.CSBK{
			LastBlock: true,
			Opcode:    pdu.CSBKPreamble,
			PreamblePDU: &pdu.PreamblePDU{
				Group:              cfg.Group,
				CSBKBlocksToFollow: byte(cfg.Preambles - i),
				TargetAddress:      cfg.Dest,
				SourceAddress:      cfg.Source,
			},
		}
		b, err := layer2.BuildBurst(c, cfg.ColorCode, cfg.source())
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	first, second, op := cfg.Source, cfg.Dest, pdu.CSBKOpcode(motoRadioCheck)
	if alert {
		first, second, op = cfg.Dest, cfg.Source, motoCallAlert
	}
	c := &pdu.CSBK{LastBlock: true, Opcode: op, FID: motoFID}
	binary.BigEndian.PutUint32(c.RawPayload[4:], second)
	binary.BigEndian.PutUint32(c.RawPayload[1:5], first)
	b, err := layer2.BuildBurst(c, cfg.ColorCode, cfg.source())
	if err != nil {
		return nil, err
	}
	return append(out, b), nil
}

// TestAMBEFrame returns frame i of the voice test pattern: a distinct,
// known 49-bit value per frame, so the relayed frames can be compared bit
// for bit.
func TestAMBEFrame(i int) vocoder.AMBEFrame {
	if i < 0 {
		i = 0
	}
	z := (uint64(i) + 1) * 0x9E3779B97F4A7C15
	z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
	z = (z ^ z>>27) * 0x94D049BB133111EB
	z ^= z >> 31
	v := z & (1<<49 - 1) << 7
	var f vocoder.AMBEFrame
	for k := range f {
		f[k] = byte(v >> (48 - 8*k) & 0xFF)
	}
	return f
}

// voiceBursts builds a voice call: the voice LC header, the PI header for
// privacy, Superframes superframes of the AMBE test pattern and the
// terminator with LC.
func voiceBursts(cfg SeqConfig, privacy bool) ([]layer2.RawBurst, []string, error) {
	lc, err := pdu.NewVoiceLC(cfg.Group, cfg.Dest, cfg.Source, layer3.ServiceOptions{IsPrivacy: privacy})
	if err != nil {
		return nil, nil, err
	}
	opts := layer2.VoiceTransmitterOptions{
		LC:        lc,
		ColorCode: cfg.ColorCode,
		Source:    cfg.source(),
		Silence:   vocoder.AMBESilenceSlot(),
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
	t, err := layer2.NewVoiceTransmitter(&opts)
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
		info = append(info, fmt.Sprintf("ambe tx seq=%d 0x%013x", i, AMBEValue(f)))
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
func rcBurst(cfg SeqConfig) (layer2.RawBurst, error) {
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
func mbcBursts(cfg SeqConfig) ([]layer2.RawBurst, []string, error) {
	h := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 0xFFF, TargetAddress: cfg.Dest, SourceAddress: cfg.Source}}
	c := pdu.CGAPContinuation{
		LastBlock: true,
		Opcode:    pdu.CSBKTalkgroupVoiceGrant,
		ColorCode: cfg.ColorCode,
		CdefParms: layer3.CdefParms{Channel: tier3Channel, TXMHz: tier3MHz, TXKHz: tier3Steps, RXMHz: tier3MHz, RXKHz: tier3Steps},
	}
	r1, err := layer2.BuildBurst(&h, cfg.ColorCode, cfg.source())
	if err != nil {
		return nil, nil, err
	}
	r2, err := layer2.BuildBurst(&c, cfg.ColorCode, cfg.source())
	if err != nil {
		return nil, nil, err
	}
	info := []string{fmt.Sprintf("mbc TV_GRANT %d -> %d channel 0xFFF, CG_AP channel %d tx/rx %d MHz + %d x 125 Hz", cfg.Source, cfg.Dest, tier3Channel, tier3MHz, tier3Steps)}
	return []layer2.RawBurst{r1, r2}, info, nil
}

// tier3Bursts returns C_ALOHA, TV_GRANT and C_MOVE CSBKs and the MBC grant.
func tier3Bursts(cfg SeqConfig) ([]layer2.RawBurst, []string, error) {
	csbks := []*pdu.CSBK{
		{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{SiteTSSync: true, NRandWait: 4, Backoff: 1, SysIdentCode: tier3SysCode, MSAddress: cfg.Dest}},
		{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: tier3Channel, TargetAddress: cfg.Dest, SourceAddress: cfg.Source}},
		{LastBlock: true, Opcode: pdu.CSBKMove, MovePDU: &pdu.MovePDU{PhysicalChannel: tier3Channel, MSAddress: cfg.Dest}},
	}
	var out []layer2.RawBurst
	for _, c := range csbks {
		b, err := layer2.BuildBurst(c, cfg.ColorCode, cfg.source())
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

func payload(name string, cfg SeqConfig) ([]layer2.RawBurst, []string, error) {
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
			return nil, nil, fmt.Errorf("voice needs -slot 0 or 1: bursts B to F are found on the 60 ms slot timing")
		}
		return voiceBursts(cfg, name == seqPI)
	case seqRC:
		if cfg.BSSourced {
			return nil, nil, fmt.Errorf("a standalone RC burst is MS-sourced")
		}
		b, err := rcBurst(cfg)
		return []layer2.RawBurst{b}, []string{"rc command=" + enums.RCCeaseTransmissionCommand.String()}, err
	case seqRadioChk, seqAlert:
		b, err := motorolaCSBKBursts(name == seqAlert, cfg)
		return b, []string{name + " fid=0x10"}, err
	case seqMBC, seqTier3:
		if !cfg.BSSourced {
			return nil, nil, fmt.Errorf("%s sends outbound Tier III PDUs and needs -bs-sourced", name)
		}
		if name == seqMBC {
			return mbcBursts(cfg)
		}
		return tier3Bursts(cfg)
	}
	b, err := DataBursts(name, cfg)
	return b, nil, err
}

// Sequence builds the bursts of a named transmission (see Sequences). An
// MS-sourced one starts with Wakeups BS_Dwn_Act CSBKs; "wakeup" sends only
// those. A BS-sourced one is a continuous stream (see BSStream), and
// "t3site" adds a second channel in Carriers (see siteSequence).
func Sequence(name string, cfg SeqConfig) (Built, error) {
	if name == seqSite {
		if !cfg.BSSourced {
			return Built{}, fmt.Errorf("%s sends outbound Tier III PDUs and needs -bs-sourced", name)
		}
		return siteSequence(cfg)
	}
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
	out := make([]layer2.RawBurst, 0, cfg.Wakeups+len(p))
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
func IdleBurst(cc enums.ColorCode) (layer2.RawBurst, error) {
	return layer2.BuildBurst(&pdu.PRFill{}, cc, layer2.SyncSourceBS)
}

// BSStream lays bursts out as a BS outbound stream: Wakeups idle frames,
// the bursts on cfg.Slot with idle bursts on the other slot, then two idle
// frames, padded to an even number of frames so the stream is a whole
// number of Short LCs. Every CACH carries a TACT with the TDMA channel of
// the burst that follows and AT set while that channel is active, and one
// row of an Activity Update Short LC, which spans four CACHs. The Short LC
// announces the activity on cfg.Slot while any of its four slots carries
// the sequence, and no activity otherwise.
func BSStream(bursts []layer2.RawBurst, cfg SeqConfig, activity enums.ActivityID) ([]BSSlot, error) {
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
			hash, err := pdu.AddressHash(cfg.Dest)
			if err != nil {
				return nil, err
			}
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

// AMBEValue returns the 49 bits of AMBE+2 frame f as a number.
func AMBEValue(f vocoder.AMBEFrame) uint64 {
	var v uint64
	for _, b := range f {
		v = v<<8 | uint64(b)
	}
	return v >> 7
}

// intList returns v as decimal numbers joined by commas.
func intList(v []int) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = strconv.Itoa(x)
	}
	return strings.Join(s, ",")
}
