package rftx

import (
	"fmt"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// SiteCarrier is a BS outbound stream on another channel of a multi-channel
// sequence. Its carrier starts LeadFrames 60 ms frames after the first
// stream and is OffsetHz from it.
type SiteCarrier struct {
	OffsetHz   float64
	LeadFrames int
	Slots      []BSSlot
}

// Signals returns fm and env as a carrier at offsetHz, followed by every
// SiteCarrier of b, modulated and delayed by its LeadFrames.
func (b Built) Signals(fm, env []float64, offsetHz float64) []Carrier {
	out := make([]Carrier, 0, 1+len(b.Carriers))
	out = append(out, Carrier{Freq: fm, Env: env, OffsetHz: offsetHz})
	for _, c := range b.Carriers {
		cf, ce := ModulateBS(c.Slots)
		lead := make([]float64, c.LeadFrames*frameSamples)
		out = append(out, Carrier{Freq: slices.Concat(lead, cf), Env: slices.Concat(lead, ce), OffsetHz: offsetHz + c.OffsetHz})
	}
	return out
}

// ETSI TS 102 361-4 Annex C.1.1.4
type siteChannel struct {
	parms  layer3.CdefParms
	offset float64
}

const (
	siteRXMHz       = 446
	siteTXMHz       = 441
	siteControlStep = 50
	siteTrafficStep = 250
	siteLeadFrames  = 30
	siteTailFrames  = 30
	siteRegrant     = 6
)

func siteSysCode() pdu.SysCode {
	return pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 21, Site: 3}
}

func siteChannels() (control, traffic siteChannel) {
	control.parms = layer3.CdefParms{Channel: 1, TXMHz: siteTXMHz, TXKHz: siteControlStep, RXMHz: siteRXMHz, RXKHz: siteControlStep}
	traffic.parms = layer3.CdefParms{Channel: 2, TXMHz: siteTXMHz, TXKHz: siteTrafficStep, RXMHz: siteRXMHz, RXKHz: siteTrafficStep}
	traffic.offset = (siteTrafficStep - siteControlStep) * 125
	return control, traffic
}

func siteBursts(cfg SeqConfig, lead, callEnd, end int) ([]layer2.RawBurst, error) {
	sys, err := siteSysCode().Pack()
	if err != nil {
		return nil, err
	}
	control, traffic := siteChannels()
	aloha := &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{
		SiteTSSync: true, ActiveConn: true, NRandWait: 4, Backoff: 1, SysIdentCode: sys}}
	var units [][]layer2.Data
	for _, ch := range []siteChannel{control, traffic} {
		h := &pdu.MBCHeader{Opcode: pdu.CSBKBroadcast, BroadcastPDU: &pdu.BroadcastPDU{
			AnnouncementType: enums.AnnouncementChanFreq, Backoff: 1, SysIdentCode: sys, BroadcastParms2: uint32(ch.parms.Channel)}}
		c := &pdu.BCAPContinuation{LastBlock: true, Opcode: pdu.CSBKBroadcast, CdefParms: ch.parms}
		units = append(units, []layer2.Data{aloha}, []layer2.Data{h, c})
	}
	grant := func(late bool) *pdu.TalkgroupVoiceGrantPDU {
		return &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: traffic.parms.Channel, LogicalChannel: true, LateEntry: late,
			TargetAddress: cfg.Dest, SourceAddress: cfg.Source}
	}
	abs := grant(false)
	abs.PhysicalChannel = 0xFFF
	var data []layer2.Data
	granted, next := false, 0
	for u := 0; len(data) < end; {
		f := len(data)
		switch {
		case !granted && f >= lead:
			data = append(data,
				&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: grant(false)},
				&pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: abs},
				&pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: cfg.ColorCode, CdefParms: traffic.parms})
			granted, next = true, f+siteRegrant
		case granted && f >= next && f < callEnd:
			data = append(data, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: grant(true)})
			next = f + siteRegrant
		default:
			data = append(data, units[u%len(units)]...)
			u++
		}
	}
	out := make([]layer2.RawBurst, 0, len(data))
	for _, d := range data {
		b, err := layer2.BuildBurst(d, cfg.ColorCode, layer2.SyncSourceBS)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// ETSI TS 102 361-4 clause 5.1.3
func controlStream(bursts []layer2.RawBurst, cc enums.ColorCode) ([]BSSlot, error) {
	idle, err := IdleBurst(cc)
	if err != nil {
		return nil, err
	}
	frames := len(bursts) + len(bursts)%2
	out := make([]BSSlot, 0, 2*frames)
	for f := range frames {
		sp := &pdu.ShortLCCSysParms{CommonSlotCounter: uint16(f / 2 % 512)}
		if err := sp.SetSysCode(siteSysCode()); err != nil {
			return nil, err
		}
		slc := pdu.ShortLC{SLCO: enums.SLCOCSysParms, CSysParms: sp}
		for ts := range 2 {
			s := BSSlot{Burst: idle}
			if ts == 0 && f < len(bursts) {
				s.Burst = bursts[f]
			}
			cach, err := layer2.EncodeShortLCToCACH(&slc, pdu.TACT{TDMAChannel: ts == 1})
			if err != nil {
				return nil, err
			}
			s.CACH = cach[len(out)%4]
			out = append(out, s)
		}
	}
	return out, nil
}

func siteSequence(cfg SeqConfig) (Built, error) {
	cfg.Group, cfg.Slot = true, 1
	voice, info, err := voiceBursts(cfg, false)
	if err != nil {
		return Built{}, err
	}
	tcfg := cfg
	tcfg.Wakeups = 2
	traffic, err := BSStream(voice, tcfg, enums.ActivityGroupVoice)
	if err != nil {
		return Built{}, err
	}
	keyUp := siteLeadFrames + 3
	callEnd := keyUp + len(traffic)/2
	bursts, err := siteBursts(cfg, siteLeadFrames, callEnd, callEnd+siteTailFrames)
	if err != nil {
		return Built{}, err
	}
	control, err := controlStream(bursts, cfg.ColorCode)
	if err != nil {
		return Built{}, err
	}
	cc, tc := siteChannels()
	sys, err := siteSysCode().Pack()
	if err != nil {
		return Built{}, err
	}
	out := make([]string, 0, 2+len(info))
	out = append(out,
		fmt.Sprintf("t3site syscode=0x%04x control lcn=%d rx=%d tx=%d offset=0, traffic lcn=%d rx=%d tx=%d offset=%+.0f",
			sys, cc.parms.Channel, cc.parms.RXFrequencyHz(), cc.parms.TXFrequencyHz(),
			tc.parms.Channel, tc.parms.RXFrequencyHz(), tc.parms.TXFrequencyHz(), tc.offset),
		fmt.Sprintf("t3site TV_GRANT %d -> TG %d lcn=%d ts=2 at %.2f s, traffic key-up at %.2f s, tune the file to %d Hz",
			cfg.Source, cfg.Dest, tc.parms.Channel, float64(siteLeadFrames)*2*slotSeconds, float64(keyUp)*2*slotSeconds, cc.parms.RXFrequencyHz()))
	return Built{
		Bursts:   append(bursts, voice...),
		Slots:    control,
		Carriers: []SiteCarrier{{OffsetHz: tc.offset, LeadFrames: keyUp, Slots: traffic}},
		Info:     append(out, info...),
	}, nil
}
