package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const repeatHold = time.Second

const rasFlag = 0x4

type pendingKind uint8

const (
	pendingNone pendingKind = iota
	pendingGrant
	pendingChanFreq
	pendingMove
)

type tsccEvent uint8

const (
	eventNone tsccEvent = iota
	eventTSCC
	eventSystem
	eventOther
	eventSiteNews
	eventGrant
	eventUDT
	eventMove
)

type control struct {
	site       Site
	plan       ChannelPlan
	hunt       HuntList
	want       pdu.SysCode
	hasWant    bool
	useHunt    bool
	b          *layer2.Burst
	opts       layer2.DecodeOptions
	slc        layer2.ShortLCAssembler
	slcOut     pdu.ShortLC
	slcP       pdu.ShortLCPayloads
	pending    pendingKind
	pendGrant  Grant
	pendChan   uint16
	pendSlot   enums.Slot
	grant      Grant
	lastGrant  Grant
	lastGrantT time.Duration
	hasLast    bool
	cs         pdu.CSBK
	counterNew bool
	acceptRAS  bool
	udt        UDT
	udtWant    int
	udtSlot    enums.Slot
	udtOn      bool
	move       pdu.MovePDU
}

func (c *control) decode(raw *layer2.RawBurst, h *layer2.Hint) bool {
	if raw == nil || c.b == nil {
		return false
	}
	c.opts.Kind = h.Kind
	c.opts.VoiceBurst = enums.VoiceBurstUnknown
	if h.Kind == layer2.KindVoice {
		c.opts.VoiceBurst = h.VoiceBurst
	}
	return c.b.DecodeFromBytesWith(*raw, &c.opts) == nil
}

func (c *control) cach(cc *layer2.CACH) bool {
	done, err := c.slc.AddInto(&c.slcOut, &c.slcP, *cc)
	if err != nil || !done || c.slcOut.FEC.Uncorrectable || c.slcOut.CSysParms == nil {
		return false
	}
	p := c.slcOut.CSysParms
	c.site.SlotCounter, c.counterNew = p.CommonSlotCounter, true
	code14 := uint16(p.MODEL)<<12 | p.NetSiteRaw
	if !c.site.HasSysCode {
		return false
	}
	if !c.site.sameSystem(code14) || c.site.Reg == p.Reg {
		return false
	}
	c.site.Reg = p.Reg
	return true
}

func (c *control) okData() bool {
	return c.dataOK() && (!c.site.HasSysCode || c.b.SlotType.ColorCode == c.site.ColorCode)
}

func (c *control) dataOK() bool {
	b := c.b
	if b.Kind != layer2.KindData || b.FEC.SlotType.Uncorrectable || b.FEC.Payload.Uncorrectable {
		return false
	}
	ras := c.acceptRAS && b.BPTCReserved&rasFlag != 0 && b.FEC.Payload.ErrorsCorrected == 0
	if b.FEC.PDU.Uncorrectable && !ras {
		return false
	}
	return b.SyncPattern == enums.SyncBSSourcedData || b.SyncPattern == enums.SyncNone
}

func (c *control) tscc(now time.Duration, slot enums.Slot) tsccEvent {
	if !c.okData() {
		return eventNone
	}
	switch d := c.b.Data.(type) {
	case *pdu.CSBK:
		c.pending = pendingNone
		if d.FID != enums.FIDStandardized {
			return eventNone
		}
		return c.csbk(now, d)
	case *pdu.MBCHeader:
		c.pending = pendingNone
		if d.FID != enums.FIDStandardized {
			return eventNone
		}
		return c.mbcHeader(d, slot)
	case *pdu.MBCContinuation:
		return c.mbcContinuation(now, d, slot)
	case *pdu.DataHeader:
		c.pending, c.udtOn = pendingNone, false
		if h := d.UDTHeader; h != nil && (h.Opcode == pdu.UDTOpcodeOutboundHeader || h.Opcode == pdu.UDTOpcodeDGNAOutboundHeader) {
			c.udt = UDT{Header: *h}
			c.udtWant, c.udtSlot, c.udtOn = int(h.AppendedBlocks)+1, slot, true
		}
		return eventTSCC
	case *pdu.Rate12Data:
		return c.udtBlock(d, slot)
	}
	return eventNone
}

func (c *control) udtBlock(d *pdu.Rate12Data, slot enums.Slot) tsccEvent {
	if !c.udtOn || slot != c.udtSlot {
		return eventNone
	}
	u := &c.udt
	u.blocks[u.n] = d.Data
	copy(u.Data[u.n*12:], d.Data[:])
	u.n++
	if u.n < c.udtWant {
		return eventTSCC
	}
	c.udtOn = false
	u.Len = u.n*12 - 2
	u.CRCOK = pdu.MBCLastBlockCRCOK(u.blocks[:u.n])
	return eventUDT
}

func (c *control) csbk(now time.Duration, d *pdu.CSBK) tsccEvent {
	switch {
	case d.AlohaPDU != nil:
		return c.aloha(d.AlohaPDU)
	case d.BroadcastPDU != nil:
		return c.broadcast(d.BroadcastPDU)
	case d.MovePDU != nil:
		if d.MovePDU.PhysicalChannel == ChannelAbsolute {
			return eventTSCC
		}
		c.move = *d.MovePDU
		return eventMove
	}
	g, ok, absolute := grantOf(d)
	if !ok {
		return eventTSCC
	}
	if absolute {
		return eventTSCC
	}
	return c.newGrant(now, &g)
}

func (c *control) system(code uint16) tsccEvent {
	var sc pdu.SysCode
	sc.Unpack(code)
	if c.hasWant && (sc.Model != c.want.Model || sc.Net != c.want.Net || sc.Site != c.want.Site) {
		return eventOther
	}
	if c.site.HasSysCode {
		if sc.Model != c.site.SysCode.Model || sc.Net != c.site.SysCode.Net || sc.Site != c.site.SysCode.Site {
			return eventOther
		}
		if sc.Par != c.site.SysCode.Par {
			c.site.SysCode.Par = sc.Par
			return eventSiteNews
		}
		return eventSystem
	}
	c.site.SysCode, c.site.HasSysCode = sc, true
	c.site.ColorCode = c.b.SlotType.ColorCode
	return eventSiteNews
}

func (c *control) aloha(a *pdu.AlohaPDU) tsccEvent {
	ev := c.system(a.SysIdentCode)
	if ev == eventOther {
		return ev
	}
	s := &c.site
	acc := Access{Mask: a.Mask, Address: a.MSAddress, Function: enums.ServiceFunction(a.ServiceFunc), NRandWait: enums.NRandWait(a.NRandWait), Backoff: enums.Backoff(a.Backoff)}
	if s.Reg != a.Reg || s.Version != a.Version || s.Offset != a.Offset || s.ActiveConn != a.ActiveConn || s.SiteTSSync != a.SiteTSSync {
		ev = eventSiteNews
	}
	s.Reg, s.Version, s.Offset, s.ActiveConn, s.SiteTSSync = a.Reg, a.Version, a.Offset, a.ActiveConn, a.SiteTSSync
	s.Access, s.HasAccess = acc, true
	return ev
}

func (c *control) broadcast(p *pdu.BroadcastPDU) tsccEvent {
	ev := c.system(p.SysIdentCode)
	if ev == eventOther {
		return ev
	}
	c.site.Access.Backoff = enums.Backoff(p.Backoff)
	if v, ok := p.AsAnnWDTSCC(); ok && c.useHunt {
		c.annWD(v.Channel1, v.ColorCode1, v.Withdraw1)
		c.annWD(v.Channel2, v.ColorCode2, v.Withdraw2)
	}
	if v, ok := p.AsAdjacentSite(); ok {
		c.site.addNeighbor(Neighbor{
			SysCode14: v.AdjacentSysCode, Channel: v.Channel, HasActiveConn: v.HasActiveConn, ActiveConn: v.ActiveConn,
			ConfirmedPriority: v.ConfirmedPriority, AdjacentPriority: v.AdjacentPriority,
		})
	}
	return ev
}

func (c *control) annWD(ch uint16, cc enums.ColorCode, withdraw bool) {
	if ch == ChannelNull {
		return
	}
	if withdraw {
		c.hunt.Withdraw(ch)
		return
	}
	_ = c.hunt.Add(HuntEntry{Channel: ch, ColorCode: cc, HasColorCode: true})
}

func (c *control) mbcHeader(h *pdu.MBCHeader, slot enums.Slot) tsccEvent {
	c.pendSlot = slot
	if h.MovePDU != nil {
		c.move, c.pending = *h.MovePDU, pendingMove
		return eventTSCC
	}
	if h.BroadcastPDU != nil {
		ev := c.system(h.BroadcastPDU.SysIdentCode)
		if v, ok := h.BroadcastPDU.AsChanFreq(); ok && ev != eventOther {
			c.pending, c.pendChan = pendingChanFreq, v.Channel
		}
		return ev
	}
	cs := &c.cs
	*cs = pdu.CSBK{Opcode: h.Opcode}
	cs.PrivateVoiceGrantPDU, cs.TalkgroupVoiceGrantPDU = h.PrivateVoiceGrantPDU, h.TalkgroupVoiceGrantPDU
	cs.BroadcastTalkgroupVoiceGrantPDU, cs.PrivateDataGrantPDU = h.BroadcastTalkgroupVoiceGrantPDU, h.PrivateDataGrantPDU
	cs.TalkgroupDataGrantPDU, cs.DuplexPrivateVoiceGrantPDU = h.TalkgroupDataGrantPDU, h.DuplexPrivateVoiceGrantPDU
	cs.DuplexPrivateDataGrantPDU, cs.PrivateDataGrantMultiItemPDU = h.DuplexPrivateDataGrantPDU, h.PrivateDataGrantMultiItemPDU
	if g, ok, _ := grantOf(cs); ok {
		c.pending, c.pendGrant = pendingGrant, g
	}
	return eventTSCC
}

func (c *control) mbcContinuation(now time.Duration, m *pdu.MBCContinuation, slot enums.Slot) tsccEvent {
	kind := c.pending
	c.pending = pendingNone
	if kind == pendingNone || slot != c.pendSlot || !m.LastBlock {
		return eventNone
	}
	switch kind {
	case pendingChanFreq:
		bc := m.AsBCAP()
		if bc.FEC.Uncorrectable || bc.Cdeftype != 0 || bc.CdefParms.Channel != c.pendChan {
			return eventNone
		}
		_ = c.plan.AddCdef(&bc.CdefParms)
		return eventTSCC
	case pendingGrant:
		cg := m.AsCGAP()
		if cg.FEC.Uncorrectable || cg.Cdeftype != 0 {
			return eventNone
		}
		_ = c.plan.AddCdef(&cg.CdefParms)
		g := c.pendGrant
		g.Channel = cg.CdefParms.Channel
		g.ColorCode, g.HasColorCode = cg.ColorCode, true
		return c.newGrant(now, &g)
	case pendingMove:
		mv := m.AsMVAP()
		if mv.FEC.Uncorrectable || mv.Cdeftype != 0 {
			return eventNone
		}
		_ = c.plan.AddCdef(&mv.CdefParms)
		c.move.PhysicalChannel = mv.CdefParms.Channel
		return eventMove
	case pendingNone:
	}
	return eventNone
}

func (c *control) newGrant(now time.Duration, g *Grant) tsccEvent {
	if ch, ok := c.plan.Lookup(g.Channel); ok && g.RXHz == 0 {
		g.RXHz, g.TXHz = ch.RXHz, ch.TXHz
	}
	repeat := c.hasLast && now-c.lastGrantT < repeatHold && sameCall(&c.lastGrant, g) && c.lastGrant.LateEntry == g.LateEntry
	c.lastGrant, c.lastGrantT, c.hasLast = *g, now, true
	if repeat {
		return eventTSCC
	}
	c.grant = *g
	return eventGrant
}

func sameCall(a, b *Grant) bool {
	return a.Opcode == b.Opcode && a.Target == b.Target && a.Source == b.Source && a.Channel == b.Channel && a.Slot == b.Slot
}

func (c *control) tsccTune() Tune {
	t := Tune{Channel: c.site.Channel, ColorCode: c.site.ColorCode, HasColorCode: c.site.HasSysCode}
	if ch, ok := c.plan.Lookup(c.site.Channel); ok {
		t.RXHz, t.TXHz = ch.RXHz, ch.TXHz
	}
	return t
}

func (c *control) huntTune(i int) Tune {
	e := c.hunt.At(i)
	t := Tune{Channel: e.Channel, ColorCode: e.ColorCode, HasColorCode: e.HasColorCode}
	if ch, ok := c.plan.Lookup(e.Channel); ok {
		t.RXHz, t.TXHz = ch.RXHz, ch.TXHz
	}
	return t
}

func (c *control) forget() {
	c.site = Site{}
	c.slc.Reset()
	c.pending = pendingNone
	c.hasLast = false
}

func slotOf(ch2 bool) enums.Slot {
	if ch2 {
		return enums.Slot2
	}
	return enums.Slot1
}

func grantOf(d *pdu.CSBK) (g Grant, ok, absolute bool) {
	g.Opcode = d.Opcode
	switch {
	case d.TalkgroupVoiceGrantPDU != nil:
		p := d.TalkgroupVoiceGrantPDU
		g.Service, g.Group = enums.ServiceKindTalkgroupVoice, true
		g.Channel, g.Slot, g.LateEntry, g.Emergency, g.Offset = p.PhysicalChannel, slotOf(p.LogicalChannel), p.LateEntry, p.Emergency, p.Offset
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.BroadcastTalkgroupVoiceGrantPDU != nil:
		p := d.BroadcastTalkgroupVoiceGrantPDU
		g.Service, g.Group = enums.ServiceKindTalkgroupVoice, true
		g.Channel, g.Slot, g.LateEntry, g.Emergency, g.Offset = p.PhysicalChannel, slotOf(p.LogicalChannel), p.LateEntry, p.Emergency, p.Offset
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.PrivateVoiceGrantPDU != nil:
		p := d.PrivateVoiceGrantPDU
		g.Service = enums.ServiceKindIndividualVoice
		g.Channel, g.Slot, g.Emergency, g.Offset = p.PhysicalChannel, slotOf(p.LogicalChannel), p.Emergency, p.Offset
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.DuplexPrivateVoiceGrantPDU != nil:
		p := d.DuplexPrivateVoiceGrantPDU
		g.Service = enums.ServiceKindDuplexVoice
		g.Channel, g.Slot, g.Emergency = p.PhysicalChannel, slotOf(p.LogicalChannel), p.Emergency
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.TalkgroupDataGrantPDU != nil:
		p := d.TalkgroupDataGrantPDU
		g.Service, g.Group = enums.ServiceKindTalkgroupPacketData, true
		g.Channel, g.Slot, g.HiRate, g.Emergency, g.Offset = p.PhysicalChannel, slotOf(p.LogicalChannel), p.HiRate, p.Emergency, p.Offset
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.PrivateDataGrantPDU != nil:
		p := d.PrivateDataGrantPDU
		g.Service = enums.ServiceKindIndividualPacketData
		g.Channel, g.Slot, g.HiRate, g.Emergency, g.Offset = p.PhysicalChannel, slotOf(p.LogicalChannel), p.HiRate, p.Emergency, p.Offset
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.PrivateDataGrantMultiItemPDU != nil:
		p := d.PrivateDataGrantMultiItemPDU
		g.Service = enums.ServiceKindIndividualPacketData
		g.Channel, g.Slot, g.HiRate, g.Emergency, g.Offset = p.PhysicalChannel, slotOf(p.LogicalChannel), p.HiRate, p.Emergency, p.Offset
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	case d.DuplexPrivateDataGrantPDU != nil:
		p := d.DuplexPrivateDataGrantPDU
		g.Service = enums.ServiceKindDuplexPacketData
		g.Channel, g.Slot, g.HiRate, g.Emergency = p.PhysicalChannel, slotOf(p.LogicalChannel), p.HiRate, p.Emergency
		g.Target, g.Source = p.TargetAddress, p.SourceAddress
	default:
		mi, isMI := d.AsTalkgroupDataGrantMultiItem()
		if !isMI || d.Hint0x38 != pdu.Opcode0x38TalkgroupDataGrantMultiItem {
			return g, false, false
		}
		g.Service, g.Group = enums.ServiceKindTalkgroupPacketData, true
		g.Channel, g.Slot, g.HiRate, g.Emergency, g.Offset = mi.PhysicalChannel, slotOf(mi.LogicalChannel), mi.HiRate, mi.Emergency, mi.Offset
		g.Target, g.Source = mi.TargetAddress, mi.SourceAddress
	}
	return g, true, g.Channel == ChannelAbsolute
}

func (s *Site) addNeighbor(n Neighbor) {
	for i := range s.NeighborsLen {
		if s.Neighbors[i].Channel == n.Channel && s.Neighbors[i].SysCode14 == n.SysCode14 {
			s.Neighbors[i] = n
			return
		}
	}
	if s.NeighborsLen < MaxNeighbors {
		s.Neighbors[s.NeighborsLen] = n
		s.NeighborsLen++
	}
}

func (s *Site) sameSystem(code14 uint16) bool {
	have, err := s.SysCode.Pack14()
	return err == nil && have == code14
}
