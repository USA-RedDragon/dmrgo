package trunk_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

type exampleSite struct {
	ms  *trunk.MS
	now time.Duration
	out []trunk.Output
}

func newExampleSite(cfg trunk.MSConfig) *exampleSite {
	cfg.Address, cfg.NoRegistration = 3120001, true
	ms, _ := trunk.NewMS(&cfg)
	s := &exampleSite{ms: ms, out: make([]trunk.Output, 0, trunk.MaxOutputs)}
	code, _ := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 1, Site: 2}.Pack()
	s.send(exampleCSBK((*pdu.CSBK).SetAloha, pdu.AlohaPDU{SysIdentCode: code}))
	return s
}

func exampleCSBK[P any](set func(*pdu.CSBK, P) error, p P) *pdu.CSBK {
	c := &pdu.CSBK{LastBlock: true}
	_ = set(c, p)
	return c
}

func (s *exampleSite) send(d layer2.Data) {
	if c, ok := d.(*pdu.CSBK); ok {
		c.LastBlock = true
	}
	raw, _ := layer2.BuildBurst(d, 1, layer2.SyncSourceBS)
	free := [3]byte{}
	s.out = s.ms.Burst(s.now, &raw, layer2.Hint{Slot: enums.Slot1}, &free, s.out[:0])
	s.now += 60 * time.Millisecond
	s.print()
}

func (s *exampleSite) print() {
	for _, o := range s.out {
		switch o.Kind {
		case trunk.OutputBurst:
			b, _ := layer2.NewBurstFromBytes(o.Burst)
			fmt.Println("send", b.SlotType.DataType)
		case trunk.OutputDelivered:
			fmt.Println(o.Kind, o.Ack)
		case trunk.OutputPolled, trunk.OutputGrant, trunk.OutputAhoy, trunk.OutputAmbient:
			fmt.Println(o.Kind, "from", o.Grant.Source)
		case trunk.OutputSite:
		default:
			fmt.Println(o.Kind)
		}
	}
}

func (s *exampleSite) udt(h pdu.UDTHeader, a *trunk.Appended) {
	h.UDTFormat, h.PadNibble, h.AppendedBlocks = a.Format, a.PadNibble, uint8(a.Blocks-1)
	if h.Opcode == 0 {
		h.Opcode = pdu.UDTOpcodeOutboundHeader
	}
	var dh pdu.DataHeader
	_ = dh.SetUDTHeader(h)
	s.send(&dh)
	blocks := make([][12]byte, a.Blocks)
	for i := range blocks {
		copy(blocks[i][:], a.Data[12*i:min(12*i+12, trunk.MaxAppendedOctets)])
	}
	pdu.SetMBCLastBlockCRC(blocks)
	for i := range blocks {
		raw, _ := layer2.BuildDataBurst(blocks[i], enums.DataTypeRate12, 1, layer2.SyncSourceBS)
		free := [3]byte{}
		s.out = s.ms.Burst(s.now, &raw, layer2.Hint{Slot: enums.Slot1}, &free, s.out[:0])
		s.now += 60 * time.Millisecond
		s.print()
	}
}

func ackd(r enums.AckReason) *pdu.CSBK {
	return exampleCSBK((*pdu.CSBK).SetAckOutbound, pdu.AckOutboundPDU{ReasonCode: byte(r), TargetAddress: 3120001})
}

func ahoyFor(kind enums.ServiceKind, src uint32, als bool) *pdu.CSBK {
	return exampleCSBK((*pdu.CSBK).SetAhoy, pdu.AhoyPDU{ALS: als, ServiceKind: uint8(kind), TargetAddress: 3120001, SourceAddress: src})
}

// ExampleMS_PollShortData polls another MS for its NMEA position: the TSCC
// delivers the polled data in a UDT, and the MS acknowledges it.
func ExampleMS_PollShortData() {
	s := newExampleSite(trunk.MSConfig{})
	_ = s.ms.PollShortData(s.now, 3120002, enums.UDTNMEA, 1)
	s.send(exampleCSBK((*pdu.CSBK).SetAloha, pdu.AlohaPDU{}))
	var a trunk.Appended
	_ = a.SetNMEAShort(&trunk.NMEAShort{North: true, Fix: true, LatDegrees: 41, LonDegrees: 87, Speed: 3})
	s.udt(pdu.UDTHeader{LLIDDestination: 3120001, LLIDSource: 3120002}, &a)
	got := s.ms.UDT().Appended()
	n, _ := got.NMEAShort()
	fmt.Println("latitude", n.LatDegrees, "longitude", n.LonDegrees, "speed", n.Speed)
	// Output:
	// send CSBK
	// UDT
	// Delivered MS_Accepted
	// send CSBK
	// latitude 41 longitude 87 speed 3
}

// ExampleMS_SetPollResponse answers a UDT poll of another MS, and a USBD
// Poll Request for LIP, with the data that the consumer set.
func ExampleMS_SetPollResponse() {
	s := newExampleSite(trunk.MSConfig{})
	var a trunk.Appended
	_ = a.SetText(enums.UDTISO8, []byte("status 7"))
	s.ms.SetPollResponse(&a)
	s.send(ahoyFor(enums.ServiceKindShortDataPolling, 3120002, false))
	lip := trunk.USBDLIPResponse{Latitude: 45, Longitude: -90}
	p, _ := lip.Encode()
	s.ms.SetUSBDResponse(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP, Payload: p})
	req := trunk.USBDPollRequest{Target: 3120001}
	q, _ := req.Encode()
	s.send(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP, Payload: q})
	// Output:
	// Polled from 3120002
	// send Data Header
	// send Rate 1/2
	// Polled from 0
	// send Unified Single Block
}

// ExampleMS_Divert diverts the voice calls of the MS to another MS: the MS
// uploads the address when the TSCC asks for it.
func ExampleMS_Divert() {
	s := newExampleSite(trunk.MSConfig{})
	var a trunk.Appended
	_ = a.SetAddresses(false, []uint32{3120005})
	_ = s.ms.Divert(s.now, constants.GatewayMSI, layer3.DiversionServiceOptions{Set: true, Voice: true}, &a)
	s.send(exampleCSBK((*pdu.CSBK).SetAloha, pdu.AlohaPDU{}))
	s.send(ahoyFor(enums.ServiceKindCallDiversion, constants.GatewayDIVERTI, false))
	s.send(ackd(enums.AckReasonMessageAccepted))
	// Output:
	// send CSBK
	// send Data Header
	// send Rate 1/2
	// Delivered Message_Accepted
}

// ExampleMS_Groups receives DGNA talkgroups and an alias, and follows a
// call to one of them.
func ExampleMS_Groups() {
	s := newExampleSite(trunk.MSConfig{})
	var a trunk.Appended
	_ = a.SetAddresses(true, []uint32{501, 502})
	s.udt(pdu.UDTHeader{LLIDDestination: 3120001, LLIDSource: 3120002, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &a)
	_ = a.SetMixed(false, 502, []byte("Fire Ops"))
	s.udt(pdu.UDTHeader{LLIDDestination: 3120001, LLIDSource: 3120002, Opcode: pdu.UDTOpcodeDGNAOutboundHeader}, &a)
	g := s.ms.Groups()
	fmt.Println(g.Groups[0], g.Groups[1], "one key", g.OneKey, "alias", string(s.ms.GroupAlias(nil, 502)))
	s.send(exampleCSBK((*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 4, TargetAddress: 502, SourceAddress: 3120002}))
	// Output:
	// UDT
	// Groups
	// send CSBK
	// UDT
	// Groups
	// send CSBK
	// 501 502 one key 501 alias Fire Ops
	// Grant from 3120002
	// Tune
}

// ExampleMS_CallDuplex makes a full-duplex call: the MS follows the
// PV_GRANT_DX that names it as the target.
func ExampleMS_CallDuplex() {
	s := newExampleSite(trunk.MSConfig{})
	_ = s.ms.CallDuplex(s.now, 3120002, layer3.VoiceServiceOptions{})
	s.send(exampleCSBK((*pdu.CSBK).SetAloha, pdu.AlohaPDU{}))
	for _, target := range []uint32{3120002, 3120001} {
		other := 3120001 + 3120002 - target
		s.send(exampleCSBK((*pdu.CSBK).SetDuplexPrivateVoiceGrant, pdu.DuplexPrivateVoiceGrantPDU{
			PhysicalChannel: 9, TargetAddress: target, SourceAddress: other}))
	}
	fmt.Println(s.ms.State())
	// Output:
	// send CSBK
	// Grant from 3120001
	// Grant from 3120002
	// Tune
	// Payload
}

// ExampleMS_CallAmbient shows the called side of an Ambient Listening
// Service call: the MS accepts the ALS radio check and reports
// OutputAmbient when it joins the call.
func ExampleMS_CallAmbient() {
	s := newExampleSite(trunk.MSConfig{Ambient: true})
	s.send(ahoyFor(enums.ServiceKindIndividualVoice, 3120002, true))
	s.send(exampleCSBK((*pdu.CSBK).SetPrivateVoiceGrant, pdu.PrivateVoiceGrantPDU{
		PhysicalChannel: 9, TargetAddress: 3120001, SourceAddress: 3120002}))
	// Output:
	// Ahoy from 3120002
	// send CSBK
	// Grant from 3120002
	// Tune
	// Ambient from 3120002
}
