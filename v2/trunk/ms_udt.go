package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/wire"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// MaxDynamicGroups is the number of DGNA talkgroups of an MS: 15 of the
// DGNA_Address mode and one of the DGNA_Alias mode (ETSI TS 102 361-4
// clause 6.6.8.0).
const MaxDynamicGroups = 16

const aliasUnits = 21

const (
	reqShortData request = iota + 16
	reqPoll
	reqDivert
	reqDGNA
)

const alsBit = 0b0010

// DynamicGroups is the DGNA talkgroup list of an MS (ETSI TS 102 361-4
// clause 6.6.8.1).
type DynamicGroups struct {
	// Groups holds ADDRESS1 to ADDRESS15 of the DGNA_Address mode in its
	// first 15 entries and the talkgroup of the DGNA_Alias mode in the
	// last. constants.AddressNull marks an unused entry.
	Groups [MaxDynamicGroups]uint32
	// OneKey is the One-key_talkgroup, when HasOneKey is set.
	OneKey uint32
	// HasOneKey reports whether the MS holds a One-key_talkgroup.
	HasOneKey bool
}

type msUDT struct {
	noDuplex bool
	ambient  bool

	reserved uint8
	proxy    bool

	send     Appended
	sendTo   uint32
	sendGrp  bool
	sendDiv  bool
	sendOp   pdu.UDTOpcode
	ahoyFrom uint32
	ahoyKind enums.ServiceKind

	poll    Appended
	hasPoll bool
	usbd    pdu.UnifiedSingleBlockData
	hasUSBD bool

	groups DynamicGroups
	alias  [MaxDynamicGroups][2 * aliasUnits]byte

	alsWait  bool
	alsEnd   time.Duration
	alsFrom  uint32
	alsCall  bool
	alsUntil time.Duration

	blk   [4][12]byte
	usbdP pdu.UnifiedSingleBlockData
}

func (u *msUDT) reset() {
	u.reserved, u.proxy, u.alsWait, u.alsCall = 0, false, false, false
	u.groups, u.alias = DynamicGroups{}, [MaxDynamicGroups][2 * aliasUnits]byte{}
}

func (u *msUDT) deadline(at time.Duration, ok bool) (time.Duration, bool) {
	if u.alsWait {
		at, ok = earliest(at, ok, u.alsEnd)
	}
	if u.alsCall {
		at, ok = earliest(at, ok, u.alsUntil)
	}
	return at, ok
}

// SendShortData asks the TSCC to deliver the appended data a as UDT short
// data to the talkgroup or MS target, or to a gateway, by random access
// (ETSI TS 102 361-4 clause 6.6.4). The MS uploads a when the TSCC asks for
// it with a C_AHOY from SDMI. The outputs follow from Burst and Tick:
// OutputDelivered when the TSCC acknowledges the delivery, or
// OutputRefused. It returns the errors of Call, and ErrUDTLength for an a
// with Blocks out of range.
func (m *MS) SendShortData(now time.Duration, group bool, target uint32, a *Appended, opts layer3.ShortDataServiceOptions) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	kind := enums.ServiceKindIndividualShortData
	if group {
		kind = enums.ServiceKindTalkgroupShortData
	}
	field, err := opts.Field()
	if err != nil {
		return errTarget
	}
	if _, err := a.size(); err != nil {
		return err
	}
	if err := m.request(now, kind, target, field, false); err != nil {
		return err
	}
	m.req = reqShortData
	m.u.reserved = uab(a.Blocks)
	m.upload(a, target, group, false, pdu.UDTOpcodeInboundHeader, constants.GatewaySDMI, enums.ServiceKindIndividualShortData)
	return nil
}

// PollShortData asks the TSCC to poll the MS target for UDT short data in
// the format f, in the given number of appended blocks, by random access
// (ETSI TS 102 361-4 clause 6.6.5). The TSCC delivers the polled data in a
// UDT, which the MS acknowledges: the outputs are OutputUDT with the data
// and OutputDelivered, or OutputRefused. It returns the errors of Call, and
// ErrUDTLength for blocks out of range.
func (m *MS) PollShortData(now time.Duration, target uint32, f enums.UDTFormat, blocks int) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	field, err := layer3.PollingServiceOptions{Format: f}.Field()
	if err != nil {
		return errTarget
	}
	if blocks < 1 || blocks > 4 {
		return ErrUDTLength
	}
	if err := m.request(now, enums.ServiceKindShortDataPolling, target, field, false); err != nil {
		return err
	}
	m.req, m.u.reserved, m.u.ahoyFrom = reqPoll, uab(blocks), 0
	return nil
}

// Divert asks the TSCC to divert the calls of the MS, or to stop diverting
// them, by random access (ETSI TS 102 361-4 clause 6.6.7.1). With opts.Set,
// gateway is MSI, GPI, PSTNI, PABXI, LINEI, DISPATI or IPI of package
// constants and a holds the diversion address: an address, BCD digits or
// an IP address. The MS uploads a when the TSCC asks for it with a C_AHOY
// from DIVERTI. Without opts.Set, gateway is TSI to clear the diversions of
// the MS or DIVERTI to clear the diversions to it, and a may be nil. The
// outputs are OutputDelivered with the C_ACKD, or OutputRefused. It returns
// the errors of Call, and a *dmrgo.ConfigError for another gateway or a missing
// a.
func (m *MS) Divert(now time.Duration, gateway uint32, opts layer3.DiversionServiceOptions, a *Appended) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	switch gateway {
	case constants.GatewayMSI, constants.GatewayGPI, constants.GatewayPSTNI, constants.GatewayPABXI, constants.GatewayLINEI,
		constants.GatewayDISPATI, constants.GatewayIPI:
		if !opts.Set || a == nil {
			return errTarget
		}
		if _, err := a.size(); err != nil {
			return err
		}
	case constants.GatewayTSI, constants.GatewayDIVERTI:
		if opts.Set {
			return errTarget
		}
	default:
		return errTarget
	}
	field, err := opts.Field()
	if err != nil {
		return errTarget
	}
	if err := m.request(now, enums.ServiceKindCallDiversion, gateway, field, false); err != nil {
		return err
	}
	m.req = reqDivert
	if opts.Set {
		m.u.reserved = uab(a.Blocks)
		m.u.proxy = a.Blocks > 1
		m.upload(a, gateway, false, true, pdu.UDTOpcodeInboundHeader, constants.GatewayDIVERTI, enums.ServiceKindCallDiversion)
	}
	return nil
}

// AssignGroups asks the TSCC to give the MS target the DGNA talkgroups of
// a, by random access to DGNAI (ETSI TS 102 361-4 clause 6.6.8.3). a is in
// the address format, for the DGNA_Address mode, or in the mixed format,
// for the DGNA_Alias mode. The MS uploads a in a C_DGNAHU when the TSCC
// asks for it with a C_AHOY from DGNAI. The outputs are OutputDelivered or
// OutputRefused. It returns the errors of Call, and ErrUDTFormat for
// another format.
func (m *MS) AssignGroups(now time.Duration, target uint32, a *Appended) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	if a == nil {
		return dmrgo.ErrNilArgument
	}
	if a.Format != enums.UDTAddress && a.Format != enums.UDTMixed {
		return ErrUDTFormat
	}
	if _, err := a.size(); err != nil {
		return err
	}
	if target == 0 || target > 0xFFFFFF {
		return errTarget
	}
	field, _ := layer3.ShortDataServiceOptions{}.Field()
	if err := m.request(now, enums.ServiceKindSupplementary, constants.GatewayDGNAI, field, false); err != nil {
		return err
	}
	m.req = reqDGNA
	m.u.reserved = uab(a.Blocks) << 2
	m.upload(a, target, false, false, pdu.UDTOpcodeDGNAInboundHeader, constants.GatewayDGNAI, enums.ServiceKindSupplementary)
	return nil
}

// CallDuplex asks the TSCC for a full-duplex voice call to the MS target,
// by random access, as Call does (ETSI TS 102 361-4 clause 6.6.9.2). The MS
// follows the PV_GRANT_DX whose Target is its own address.
func (m *MS) CallDuplex(now time.Duration, target uint32, opts layer3.VoiceServiceOptions) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	field, err := opts.Field()
	if err != nil {
		return errTarget
	}
	return m.request(now, enums.ServiceKindDuplexVoice, target, field, opts.IsEmergency)
}

// CallDuplexData asks the TSCC for a full-duplex packet data call to the MS
// target, by random access, as CallData does (ETSI TS 102 361-4 clause
// 6.6.10.2).
func (m *MS) CallDuplexData(now time.Duration, target uint32, opts layer3.DataServiceOptions) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	field, err := opts.Field()
	if err != nil {
		return errTarget
	}
	return m.request(now, enums.ServiceKindDuplexPacketData, target, field, opts.IsEmergency)
}

// CallAmbient asks the TSCC for an individual voice call to the MS target
// with the Ambient Listening Service, by random access, as Call does: the
// called MS transmits without an action of its user (ETSI TS 102 361-4
// clause 6.6.2.6.2).
func (m *MS) CallAmbient(now time.Duration, target uint32, opts layer3.VoiceServiceOptions) error {
	if m == nil {
		return dmrgo.ErrNilReceiver
	}
	if m.f.ctl.b == nil {
		return dmrgo.ErrZeroValue
	}

	if err := m.Call(now, false, target, opts); err != nil {
		return err
	}
	m.u.reserved = alsBit
	return nil
}

// SetPollResponse sets the data that the MS sends when the TSCC polls it
// for UDT short data with a C_AHOY. A nil a clears it, and the MS then
// refuses a poll with C_NACKU MSNot_Supported. The MS keeps a copy. (ETSI
// TS 102 361-4 clause 6.6.5.7)
func (m *MS) SetPollResponse(a *Appended) {
	if m == nil {
		return
	}

	m.u.hasPoll = a != nil
	if a != nil {
		m.u.poll = *a
	}
}

// SetUSBDResponse sets the Poll Response that the MS sends to a USBD Poll
// Request of the same Service Type for it, such as a USBDLIPResponse in
// its Payload. A nil p clears it, and the MS then refuses a poll with
// C_NACKU MSNot_Supported. The MS keeps a copy. (ETSI TS 102 361-4 clause
// 6.6.11.2)
func (m *MS) SetUSBDResponse(p *pdu.UnifiedSingleBlockData) {
	if m == nil {
		return
	}

	m.u.hasUSBD = p != nil
	if p != nil {
		m.u.usbd = *p
	}
}

// Groups returns the DGNA talkgroups of the MS. The MS follows their calls
// as those of MSConfig.Talkgroups. (ETSI TS 102 361-4 clause 6.6.8.1)
func (m *MS) Groups() DynamicGroups {
	if m == nil {
		return DynamicGroups{}
	}

	return m.u.groups
}

// GroupAlias appends the DGNA alias of the talkgroup tg to dst as UTF-8,
// without trailing DEL characters, and returns dst; it appends nothing for
// a talkgroup without an alias. It allocates only when dst has no room for
// 63 more octets. (ETSI TS 102 361-4 clause 6.6.8.1.2)
func (m *MS) GroupAlias(dst []byte, tg uint32) []byte {
	if m == nil {
		return dst
	}

	for i, g := range m.u.groups.Groups {
		if g != constants.AddressNull && g == tg {
			return appendAlias(dst, &m.u.alias[i])
		}
	}
	return dst
}

func appendAlias(dst []byte, a *[2 * aliasUnits]byte) []byte {
	n := 0
	for n < aliasUnits && (a[2*n] != 0 || a[2*n+1] != 0) {
		n++
	}
	return trimDEL(wire.AppendUTF8(dst, a[:2*n], true), len(dst))
}

func uab(blocks int) uint8 {
	if blocks < 1 || blocks > 4 {
		return 0
	}
	return uint8(blocks - 1)
}

func (m *MS) upload(a *Appended, to uint32, group, div bool, op pdu.UDTOpcode, from uint32, kind enums.ServiceKind) {
	m.u.send, m.u.sendTo, m.u.sendGrp, m.u.sendDiv, m.u.sendOp = *a, to, group, div, op
	m.u.ahoyFrom, m.u.ahoyKind = from, kind
}

func (m *MS) udtRequest() bool {
	return m.req >= reqShortData
}

func (m *MS) staleAck(from uint32) bool {
	return m.udtRequest() && from != 0 && from != m.target && from != m.u.sendTo
}

func (m *MS) ahoyAny(now time.Duration, a *pdu.AhoyPDU, h *layer2.Hint, out []Output) []Output {
	if a.Group || a.TargetAddress != m.addr {
		return m.ahoy(now, a, h, out)
	}
	slot := h.Slot
	if slot == enums.SlotUnknown {
		slot = m.slot
	}
	kind := enums.ServiceKind(a.ServiceKind)
	switch {
	case m.udtRequest() && m.u.ahoyFrom != 0 && a.SourceAddress == m.u.ahoyFrom &&
		(kind == m.u.ahoyKind || m.req == reqDivert):
		m.ra, m.waitAnswer, m.reqUntil = raIdle, true, now+m.answerTO()
		return m.sendUDT(now+frame, slot, &m.u.send, m.u.sendTo, m.u.sendGrp, m.u.sendDiv, false, m.u.sendOp, out)
	case kind == enums.ServiceKindShortDataPolling && m.req == reqNone:
		out = append(out, Output{Kind: OutputPolled, Grant: Grant{Service: kind, Target: m.addr, Source: a.SourceAddress}})
		if !m.u.hasPoll {
			return m.ackU(now+frame, slot, enums.AckReasonMSNotSupported, a.SourceAddress, out)
		}
		return m.sendUDT(now+frame, slot, &m.u.poll, a.SourceAddress, false, false, false, pdu.UDTOpcodeInboundHeader, out)
	case kind == enums.ServiceKindDuplexVoice && m.req == reqNone:
		out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: kind, Target: a.TargetAddress, Source: a.SourceAddress}})
		if m.u.noDuplex {
			return m.ackU(now+frame, slot, enums.AckReasonMSDuplexNotSupported, a.SourceAddress, out)
		}
		m.pending, m.pendingEnd = true, now+m.pendingTO
		return m.ackU(now+frame, slot, enums.AckReasonMSAccepted, a.SourceAddress, out)
	case kind == enums.ServiceKindIndividualVoice && a.ALS && m.req == reqNone:
		out = append(out, Output{Kind: OutputAhoy, Grant: Grant{Service: kind, Target: a.TargetAddress, Source: a.SourceAddress}})
		if !m.u.ambient {
			return m.ackU(now+frame, slot, enums.AckReasonMSNotSupported, a.SourceAddress, out)
		}
		m.u.alsWait, m.u.alsEnd, m.u.alsFrom = true, now+constants.TALSRequestLifeSpanDefault, a.SourceAddress
		m.pending, m.pendingEnd = true, now+m.pendingTO
		return m.ackU(now+frame, slot, enums.AckReasonMessageAccepted, a.SourceAddress, out)
	}
	return m.ahoy(now, a, h, out)
}

func (m *MS) udtAnswer(now time.Duration, r enums.AckReason, out []Output) []Output {
	switch r.Type() {
	case enums.AckNACK:
		return m.fail(ReasonNack, r, out)
	case enums.AckACK:
		m.done()
		return append(out, Output{Kind: OutputDelivered, Ack: r})
	case enums.AckQACK, enums.AckWACK:
	}
	m.waitAnswer, m.reqUntil = true, now+m.answerTO()
	return out
}

func (m *MS) udtIn(now time.Duration, out []Output) []Output {
	u := &m.f.ctl.udt
	h := &u.Header
	if !u.CRCOK || h.Group || h.LLIDDestination != m.addr {
		return out
	}
	slot := m.f.ctl.udtSlot
	if slot == enums.SlotUnknown {
		slot = m.slot
	}
	switch {
	case h.Opcode == pdu.UDTOpcodeDGNAOutboundHeader:
		a := u.Appended()
		if !m.assign(&a) {
			return out
		}
		out = append(out, Output{Kind: OutputGroups})
		return m.ackU(now+frame, slot, enums.AckReasonMSAccepted, h.LLIDSource, out)
	case h.SupplementaryFlag && h.LLIDSource == constants.GatewayDIVERTI:
		if m.req != reqNone && m.req != reqDereg && m.req != reqRegister {
			return m.fail(ReasonDiverted, 0, out)
		}
		return out
	case m.req == reqPoll && h.LLIDSource == m.target:
		m.done()
		out = append(out, Output{Kind: OutputDelivered, Ack: enums.AckReasonMSAccepted})
		return m.ackU(now+frame, slot, enums.AckReasonMSAccepted, h.LLIDSource, out)
	case h.SupplementaryFlag:
		return out
	}
	return m.ackU(now+frame, slot, enums.AckReasonMSAccepted, h.LLIDSource, out)
}

func (m *MS) assign(a *Appended) bool {
	g := &m.u.groups
	switch a.Format {
	case enums.UDTAddress:
		l, err := a.Addresses()
		if err != nil {
			return false
		}
		alias := m.u.alias[MaxDynamicGroups-1]
		last := g.Groups[MaxDynamicGroups-1]
		oneKey, hasOneKey := g.OneKey, g.HasOneKey && g.OneKey == last
		*g = DynamicGroups{}
		m.u.alias = [MaxDynamicGroups][2 * aliasUnits]byte{}
		copy(g.Groups[:], l.Addresses[:l.Len])
		g.Groups[MaxDynamicGroups-1], m.u.alias[MaxDynamicGroups-1] = last, alias
		g.OneKey, g.HasOneKey = oneKey, hasOneKey
		if l.OneKey && l.Addresses[0] != constants.AddressNull {
			g.OneKey, g.HasOneKey = l.Addresses[0], true
		}
	case enums.UDTMixed:
		addr, oneKey, err := a.Mixed()
		if err != nil || 4*int(a.PadNibble) > appendedBits(a.Blocks)-32 {
			return false
		}
		i := MaxDynamicGroups - 1
		for k := range MaxDynamicGroups - 1 {
			if addr != constants.AddressNull && g.Groups[k] == addr {
				i = k
			}
		}
		g.Groups[i] = addr
		m.u.alias[i] = [2 * aliasUnits]byte{}
		n := min((appendedBits(a.Blocks)-32-4*int(a.PadNibble))/16, aliasUnits)
		copy(m.u.alias[i][:], a.Data[4:4+2*n])
		if oneKey && addr != constants.AddressNull {
			g.OneKey, g.HasOneKey = addr, true
		}
	default:
		return false
	}
	m.f.dyn = g.Groups
	return true
}

func (m *MS) joined(now time.Duration, out []Output) []Output {
	g := &m.f.call
	if g.Service == enums.ServiceKindDuplexVoice || g.Service == enums.ServiceKindDuplexPacketData {
		m.ownCall = true
	}
	if !m.u.alsWait || g.Service != enums.ServiceKindIndividualVoice || g.Target != m.addr || g.Source != m.u.alsFrom {
		return out
	}
	m.u.alsWait, m.u.alsCall, m.ownCall = false, true, true
	m.u.alsUntil = now + constants.TALSDefault
	if g.Emergency {
		m.u.alsUntil = now + constants.TALSEDefault
	}
	return append(out, Output{Kind: OutputAmbient, Grant: *g})
}

func (m *MS) udtTimers(now time.Duration, out []Output) []Output {
	if m.u.alsWait && now >= m.u.alsEnd {
		m.u.alsWait = false
	}
	if !m.u.alsCall {
		return out
	}
	if m.f.state != StatePayload {
		m.u.alsCall = false
		return out
	}
	if now >= m.u.alsUntil {
		m.u.alsCall = false
		return m.EndCall(now, out)
	}
	return out
}

func (m *MS) usbdPoll(now time.Duration, p *pdu.UnifiedSingleBlockData, h *layer2.Hint, out []Output) []Output {
	if p.FEC.Uncorrectable {
		return out
	}
	r := DecodeUSBDPollRequest(p.Payload)
	if r.Target != m.addr {
		return out
	}
	slot := h.Slot
	if slot == enums.SlotUnknown {
		slot = m.slot
	}
	due := now + frame*time.Duration(1+r.ResponseDelay)
	if m.f.ctl.site.Offset {
		due += slotLength
	}
	out = append(out, Output{Kind: OutputPolled, Grant: Grant{Service: enums.ServiceKindShortDataPolling, Target: m.addr}})
	if !m.u.hasUSBD || m.u.usbd.ServiceType != p.ServiceType {
		return m.ackU(due, slot, enums.AckReasonMSNotSupported, constants.GatewayTSI, out)
	}
	m.u.usbdP = m.u.usbd
	b, err := m.u.usbdP.Encode()
	if err != nil {
		return out
	}
	return m.sendRaw(due, slot, b, enums.DataTypeUnifiedSingleBlock, out)
}

func (m *MS) ackU(due time.Duration, slot enums.Slot, r enums.AckReason, to uint32, out []Output) []Output {
	return m.sendAck(due, slot, pdu.AckInboundPDU{ReasonCode: byte(r), TargetAddress: to, AdditionalInfo: m.addr}, out)
}

func (m *MS) sendUDT(due time.Duration, slot enums.Slot, a *Appended, to uint32, group, div, supp bool, op pdu.UDTOpcode, out []Output) []Output {
	u := &m.u
	var dh pdu.DataHeader
	err := dh.SetUDT(pdu.UDTHeader{
		Group: group, ResponseRequested: op == pdu.UDTOpcodeDGNAInboundHeader, OptionFlag: div, SAP: enums.SAPUnifiedDataTransport,
		UDTFormat: a.Format, LLIDDestination: to, LLIDSource: m.addr, PadNibble: a.PadNibble, UAB: uab(a.Blocks), SupplementaryFlag: supp,
		Opcode: op,
	})
	if err != nil {
		return out
	}
	p, err := dh.Encode()
	if err != nil {
		return out
	}
	u.blk = [4][12]byte{}
	for i := range a.Blocks {
		copy(u.blk[i][:], a.Data[12*i:min(12*i+12, MaxAppendedOctets)])
	}
	pdu.SetMBCLastBlockCRC(u.blk[:a.Blocks])
	out = m.sendRaw(due, slot, p, enums.DataTypeDataHeader, out)
	for i := range a.Blocks {
		out = m.sendRaw(due+time.Duration(i+1)*frame, slot, u.blk[i], enums.DataTypeRate12, out)
	}
	return out
}

func (m *MS) sendRaw(due time.Duration, slot enums.Slot, p [12]byte, dt enums.DataType, out []Output) []Output {
	cc := m.f.ctl.site.ColorCode
	if m.f.state == StatePayload && m.f.call.HasColorCode {
		cc = m.f.call.ColorCode
	}
	raw, err := layer2.BuildDataBurst(p, dt, cc, layer2.SyncSourceMS)
	if err != nil {
		return out
	}
	h := layer2.Hint{Slot: slot, Kind: layer2.KindData, DataType: dt, HasDataType: true}
	return append(out, Output{Kind: OutputBurst, Burst: raw, Hint: h, Slot: slot, Due: due})
}
