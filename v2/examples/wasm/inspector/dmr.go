package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

type stream struct {
	asm *packet.Assembler
	elc layer2.EmbeddedLCAssembler
}

type session struct {
	proto       string
	dialect     dmrd.Dialect
	payloadBase int
	streams     map[string]*stream
	loc         location.Decoder
	lrrp        lrrp.Message
}

func newSession(proto string) *session {
	if proto == "auto" {
		proto = ""
	}
	return &session{proto: proto, streams: map[string]*stream{}}
}

func (s *session) stream(key string) *stream {
	st := s.streams[key]
	if st == nil {
		st = &stream{asm: packet.NewAssembler(127)}
		st.asm.AcceptBadCRC = true
		s.streams[key] = st
	}
	return st
}

func extractBits(raw []byte, ranges [][2]int) []byte {
	n := 0
	for _, r := range ranges {
		n += r[1] - r[0]
	}
	out := make([]byte, (n+7)/8)
	k := 0
	for _, r := range ranges {
		for i := r[0]; i < r[1]; i++ {
			if raw[i/8]&(0x80>>(i%8)) != 0 {
				out[k/8] |= 0x80 >> (k % 8)
			}
			k++
		}
	}
	return out
}

func bitRanges(buf, base int, rs ...[2]int) []Range {
	out := make([]Range, 0, len(rs))
	for _, r := range rs {
		out = append(out, Range{Buf: buf, Start: base + r[0], End: base + r[1]})
	}
	return out
}

func payloadBits() [][2]int  { return [][2]int{{0, 98}, {166, 264}} }
func slotTypeBits() [][2]int { return [][2]int{{98, 108}, {156, 166}} }
func embBits() [][2]int      { return [][2]int{{108, 116}, {148, 156}} }

func voiceSlotBits(i int) [][2]int {
	return [3][][2]int{{{0, 72}}, {{72, 108}, {156, 192}}, {{192, 264}}}[i]
}

func (s *session) decodeBurst(pb *packetBuilder, raw layer2.RawBurst, buf, base int, key string, opts *layer2.DecodeOptions) *Node {
	n := &Node{Name: burstNode, Ranges: bitRanges(buf, base, [2]int{0, 264})}
	b := layer2.NewBurst()
	var err error
	if opts != nil {
		err = b.DecodeFromBytesWith(raw, opts)
	} else {
		err = b.DecodeFromBytes(raw)
	}
	if err != nil {
		n.Err = "burst decode: " + err.Error()
		pb.errorf("%s: %v", burstNode, err)
	}
	n.Value = b.Kind.String()
	add := func(c *Node) { n.Children = append(n.Children, c) }
	add(&Node{Name: "Kind", Value: b.Kind.String()})
	if b.SyncPattern != enums.SyncNone {
		add(&Node{Name: "SYNC", Value: fmt.Sprintf("%s, distance %d", b.SyncPattern, b.SyncDistance), Ranges: bitRanges(buf, base, [2]int{108, 156})})
	} else {
		add(&Node{Name: "Nearest SYNC", Value: fmt.Sprintf("%s, distance %d (center is embedded signalling)", b.NearestSync, b.SyncDistance)})
	}
	switch b.Kind {
	case layer2.KindVoice:
		s.voiceNodes(pb, n, b, buf, base, key)
	case layer2.KindData:
		s.dataNodes(pb, n, b, raw, buf, base, key)
	case layer2.KindReverseChannel:
		add(&Node{Name: "Reverse Channel", Value: b.ReverseChannel.String(), Children: walkPlain(reflect.ValueOf(b.ReverseChannel))})
	default:
	}
	fecNode := &Node{Name: "FEC", Value: fecText(b.FEC.Aggregate()), Children: walkPlain(reflect.ValueOf(b.FEC))}
	if b.FEC.Aggregate().Uncorrectable {
		fecNode.Err = "an FEC or CRC check of the burst failed"
		pb.errorf("%s: FEC or CRC check failed", burstNode)
	}
	add(fecNode)
	return n
}

func (s *session) embNodes(pb *packetBuilder, n *Node, b *layer2.Burst, buf, base int) {
	if !b.HasEmbeddedSignalling() {
		return
	}
	raw, _ := b.Encode()
	eb := pb.buffer("EMB (16 bits)", extractBits(raw[:], embBits()), -1, nil)
	e := &Node{Name: "EMB", Value: b.EmbeddedSignalling.String(), Ranges: bitRanges(buf, base, embBits()...)}
	e.Children = walkBits(reflect.ValueOf(b.EmbeddedSignalling), eb, 0)
	n.Children = append(n.Children, e,
		&Node{Name: "Embedded data", Value: hex.EncodeToString(b.EmbeddedSignallingData[:]), Ranges: bitRanges(buf, base, [2]int{116, 148})})
}

func (s *session) voiceNodes(pb *packetBuilder, n *Node, b *layer2.Burst, buf, base int, key string) {
	if b.VoiceBurst != enums.VoiceBurstUnknown {
		n.Children = append(n.Children, &Node{Name: "Voice position", Value: b.VoiceBurst.String()})
	}
	_, res := b.AMBEFrames()
	for i := range b.VoiceSlots {
		n.Children = append(n.Children, &Node{Name: fmt.Sprintf("Vocoder slot %d", i+1), Value: hex.EncodeToString(b.VoiceSlots[i][:]), Ranges: bitRanges(buf, base, voiceSlotBits(i)...)})
	}
	vn := &Node{Name: "AMBE+2 FEC", Value: fecText(res)}
	if res.Uncorrectable {
		vn.Err = "an AMBE+2 frame has more errors than its FEC corrects"
	}
	n.Children = append(n.Children, vn)
	s.embNodes(pb, n, b, buf, base)
	if b.HasEmbeddedSignalling() {
		st := s.stream(key)
		lc, _, done, err := st.elc.Add(b.EmbeddedSignallingData, b.EmbeddedSignalling.LCSS)
		if err != nil {
			n.Children = append(n.Children, &Node{Name: "Embedded LC", Err: err.Error()})
		} else if done {
			n.Children = append(n.Children, s.lcNode(pb, "Embedded LC (assembled)", lc, -1))
		}
	}
}

func (s *session) lcNode(pb *packetBuilder, name string, lc [9]byte, at int) *Node {
	var m []int
	parent := -1
	if at >= 0 {
		parent = 0
		for i := range lc {
			m = append(m, at+i)
		}
	}
	lb := pb.buffer(name+" (72 bits)", lc[:], parent, m)
	flc := pdu.DecodeFullLinkControlBytes(lc)
	n := &Node{Name: name, Ranges: []Range{{Buf: lb, Start: 0, End: 72}}}
	n.Value = flc.String()
	n.Children = walkBits(reflect.ValueOf(&flc), lb, 0)
	n.Children = append(n.Children, capPlusLC(&flc, lb)...)
	if t, src, ok := flc.Addresses(); ok {
		pb.summary("LC %d > %d", src, t)
	}
	return n
}

func (s *session) dataNodes(pb *packetBuilder, n *Node, b *layer2.Burst, raw layer2.RawBurst, buf, base int, key string) {
	stb := pb.buffer("slot type (20 bits)", extractBits(raw[:], slotTypeBits()), -1, nil)
	st := &Node{Name: "Slot type", Value: b.SlotType.String(), Ranges: bitRanges(buf, base, slotTypeBits()...)}
	st.Children = walkBits(reflect.ValueOf(&b.SlotType), stb, 0)
	if b.SlotType.FEC.Uncorrectable {
		st.Err = "Golay(20,8) of the slot type failed"
	}
	n.Children = append(n.Children, st)
	s.embNodes(pb, n, b, buf, base)
	dt := b.SlotType.DataType
	var payload []byte
	name := "BPTC payload (96 bits)"
	switch dt {
	case enums.DataTypeRate34:
		p, _ := raw.Rate34Payload()
		payload, name = p[:], "rate 3/4 payload (144 bits)"
	case enums.DataTypeRate1:
		p, _ := raw.Rate1Payload()
		payload, name = p[:], "rate 1 payload (192 bits)"
	default:
		p, _ := raw.BPTCPayload()
		payload = p[:]
	}
	var pm []int
	parent := -1
	if s.payloadBase > 0 {
		parent = 0
		for i := range payload {
			pm = append(pm, s.payloadBase+i)
		}
	}
	pbuf := pb.buffer(name, payload, parent, pm)
	pn := &Node{Name: "Payload", Value: name, Ranges: bitRanges(buf, base, payloadBits()...)}
	n.Children = append(n.Children, pn)
	pb.summary("%s CC %d", dt, b.SlotType.ColorCode)
	if b.Data == nil {
		pn.Err = "no PDU: reserved data type"
		return
	}
	n.Children = append(n.Children, s.pduNode(pb, b.Data, pbuf, 0))
	s.assemble(pb, n, b, key)
}

func (s *session) pduNode(pb *packetBuilder, d layer2.Data, buf, base int) *Node {
	v := reflect.ValueOf(d)
	n := &Node{Name: reflect.Indirect(v).Type().Name(), Value: d.String()}
	n.Children = walkBits(v, buf, base)
	n.Ranges = []Range{{Buf: buf, Start: base, End: base + 96}}
	switch p := d.(type) {
	case *pdu.CSBK:
		n.Children = append(n.Children, csbkExtras(p, buf, base)...)
		if p.FEC.Uncorrectable {
			n.Err = "CSBK CRC failed"
		}
	case *pdu.FullLinkControl:
		n.Children = append(n.Children, capPlusLC(p, buf)...)
		if t, src, ok := p.Addresses(); ok {
			pb.summary("LC %d > %d", src, t)
		}
		if p.FEC.Uncorrectable {
			n.Err = "RS(12,9) of the LC failed"
		}
	case *pdu.DataHeader:
		if t, src, ok := p.Addresses(); ok {
			pb.summary("data %d > %d", src, t)
		}
		if p.FEC.Uncorrectable {
			n.Err = "data header CRC failed"
		}
	}
	return n
}

func csbkExtras(c *pdu.CSBK, buf, base int) []*Node {
	var out []*Node
	add := func(name string, v any, ok bool) {
		if ok {
			rv := reflect.ValueOf(v)
			out = append(out, &Node{Name: name, Value: fmt.Sprintf("%+v", v), Children: walkBits(rv, buf, base+16), Ranges: []Range{{Buf: buf, Start: base + 16, End: base + 80}}})
		}
	}
	if p, ok := c.AsBSOutboundActivation(); ok {
		add("BS_Dwn_Act", p, ok)
	}
	if p, ok := c.AsTalkgroupDataGrantMultiItem(); ok {
		add("TD_GRANT_MI", p, ok)
	}
	if p, ok := c.AsMotorolaRadioCheck(); ok {
		add("Motorola radio check", p, ok)
	}
	if p, ok := c.AsMotorolaCallAlert(); ok {
		add("Motorola call alert", p, ok)
	}
	if p, ok := c.AsMotorolaCallAlertAck(); ok {
		add("Motorola call alert ack", p, ok)
	}
	if p, ok := c.AsCapacityPlusSiteStatus(); ok {
		add("Capacity Plus site status", p, ok)
	}
	if p, ok := c.AsCapacityPlusNeighbors(); ok {
		add("Capacity Plus neighbors", p, ok)
	}
	if p, ok := c.AsCapacityPlusPreamble(); ok {
		add("Capacity Plus preamble", p, ok)
	}
	if p, ok := c.AsCapacityPlusDataRevertAnnouncement(); ok {
		add("Capacity Plus data revert announcement", p, ok)
	}
	if p, ok := c.AsCapacityPlusDataRevertGrant(); ok {
		add("Capacity Plus data revert grant", p, ok)
	}
	return out
}

func capPlusLC(lc *pdu.FullLinkControl, buf int) []*Node {
	var out []*Node
	if p, ok := lc.AsCapacityPlusGroupVoiceGrant(); ok {
		out = append(out, &Node{Name: "Capacity Plus group voice grant", Value: fmt.Sprintf("%+v", p), Children: walkBits(reflect.ValueOf(p), buf, 16)})
	}
	if p, ok := lc.AsCapacityPlusPrivateVoiceGrant(); ok {
		out = append(out, &Node{Name: "Capacity Plus private voice grant", Value: fmt.Sprintf("%+v", p), Children: walkBits(reflect.ValueOf(p), buf, 16)})
	}
	return out
}

func (s *session) assemble(pb *packetBuilder, n *Node, b *layer2.Burst, key string) {
	switch b.SlotType.DataType {
	case enums.DataTypeDataHeader, enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
	default:
		return
	}
	st := s.stream(key)
	m, done, err := st.asm.AddBurst(b)
	if err != nil {
		n.Children = append(n.Children, &Node{Name: "Data packet", Err: "reassembly: " + err.Error()})
		pb.errorf("data packet: %v", err)
	}
	if done {
		n.Children = append(n.Children, s.messageNode(pb, &m))
	}
}

func sapOf(h *pdu.DataHeader) (enums.SAP, bool) {
	return h.SAP()
}

func (s *session) messageNode(pb *packetBuilder, m *packet.Message) *Node {
	ub := pb.buffer("user data", m.Data, -1, nil)
	n := &Node{Name: "Data packet (reassembled)", Value: fmt.Sprintf("%s, %d octets, CRC %08X", m.DataType, len(m.Data), m.CRC)}
	if len(m.Data) > 0 {
		n.Ranges = []Range{{Buf: ub, Start: 0, End: len(m.Data) * 8}}
	}
	n.Children = append(n.Children, &Node{Name: "Header", Value: m.Header.String(), Children: walkPlain(reflect.ValueOf(&m.Header))})
	if m.BadCRC {
		n.Err = "the message CRC failed"
		pb.errorf("data packet: message CRC failed")
	}
	if t, src, ok := m.Header.Addresses(); ok {
		pb.summary("packet %d > %d, %d octets", src, t, len(m.Data))
	}
	if m.Header.Format() == pdu.FormatResponsePacket {
		if m.Response != nil {
			n.Children = append(n.Children, &Node{Name: "Response data", Value: fmt.Sprintf("retry blocks %v", m.Response.RetryBlocks()), Children: walkPlain(reflect.ValueOf(m.Response))})
		}
		rh, _ := m.Header.AsResponse()
		if r, err := packet.ResponseOf(&rh); err == nil {
			n.Children = append(n.Children, &Node{Name: "Response", Value: r.String()})
			pb.summary("response %s", r)
		}
		return n
	}
	n.Children = append(n.Children, s.appNodes(pb, &m.Header, m.Data, ub)...)
	return n
}

func (s *session) appNodes(pb *packetBuilder, h *pdu.DataHeader, data []byte, ub int) []*Node {
	var out []*Node
	if _, t, err := sms.Decode(nil, h, data); err == nil {
		out = append(out, &Node{Name: "Text message", Value: fmt.Sprintf("%q", t.Body), Children: walkPlain(reflect.ValueOf(t))})
		pb.summary("text %q", t.Body)
	} else if !errors.Is(err, sms.ErrNotText) {
		out = append(out, &Node{Name: "Text message", Err: err.Error()})
		pb.errorf("text message: %v", err)
	}
	if fix, err := s.loc.Decode(h, data); err == nil {
		out = append(out, &Node{Name: "Position", Value: fmt.Sprintf("%.6f, %.6f", fix.Latitude, fix.Longitude), Children: walkPlain(reflect.ValueOf(fix))})
		pb.summary("position %.5f, %.5f", fix.Latitude, fix.Longitude)
	} else if !errors.Is(err, location.ErrNoFix) {
		out = append(out, &Node{Name: "Position", Value: "not a position", Err: err.Error()})
	}
	sap, ok := sapOf(h)
	if !ok {
		return out
	}
	dg, err := ip.Decode(sap, data)
	if err != nil {
		if sap == enums.SAPIPBasedPacketData || sap == enums.SAPUDPIPHeaderCompression {
			out = append(out, &Node{Name: "UDP datagram", Err: err.Error()})
			pb.errorf("UDP datagram: %v", err)
		}
		return out
	}
	po, _ := slicePos(reflect.ValueOf(dg.Payload), data)
	if len(dg.Payload) == 0 {
		po = len(data)
	}
	un := &Node{Name: "UDP datagram", Value: fmt.Sprintf("port %d > %d", dg.SourcePort, dg.DestinationPort), Ranges: []Range{{Buf: ub, Start: 0, End: po * 8}}}
	un.Children = walkPlain(reflect.ValueOf(dg))
	if pn := findNode(un.Children, "Payload"); pn != nil {
		pn.Ranges = []Range{{Buf: ub, Start: po * 8, End: len(data) * 8}}
	}
	out = append(out, un)
	out = append(out, appPort(pb, int(dg.DestinationPort), dg.Payload, ub, po, &s.lrrp)...)
	return out
}

func appPort(pb *packetBuilder, port int, data []byte, buf, off int, lm *lrrp.Message) []*Node {
	var out []*Node
	switch {
	case port == int(ip.PortTMS):
		m, err := tms.DecodeMessage(data)
		out = append(out, appResult("MOTOTRBO TMS", m, err, data, buf, off))
	case port == int(ip.PortARS):
		m, err := ars.DecodeMessage(data)
		out = append(out, appResult("MOTOTRBO ARS", m, err, data, buf, off))
	case port == int(ip.PortLRRP):
		err := lrrp.DecodeMessageInto(lm, data)
		n := appResult("MOTOTRBO LRRP", lm, err, data, buf, off)
		out = append(out, n)
	case port >= 3002 && port <= 3009:
		out = append(out, hdapNode(pb, data, buf, off))
	}
	return out
}

func appResult(name string, v any, err error, data []byte, buf, off int) *Node {
	n := &Node{Name: name, Ranges: []Range{{Buf: buf, Start: off * 8, End: (off + len(data)) * 8}}}
	if err != nil {
		n.Err = err.Error()
		return n
	}
	if s, ok := v.(fmt.Stringer); ok {
		n.Value = s.String()
	}
	n.Children = walkStream(reflect.ValueOf(v), data, buf, off)
	return n
}

func hdapNode(pb *packetBuilder, data []byte, buf, off int) *Node {
	m, err := hdap.DecodeMessage(data)
	n := appResult("Hytera HDAP", &m, err, data, buf, off)
	if err != nil {
		pb.errorf("HDAP: %v", err)
		return n
	}
	if !m.ChecksumOK() {
		if c := findNode(n.Children, "Checksum"); c != nil {
			c.Err = "checksum does not match"
		}
		pb.errorf("HDAP: checksum does not match")
	}
	po, _ := slicePos(reflect.ValueOf(m.Payload), data)
	var v any
	var perr error
	switch m.Header.Service {
	case hdap.ServiceRRS:
		switch m.Opcode {
		case hdap.RRSRegistrationAnswer:
			v, perr = hdap.DecodeRRSAnswer(m.Payload)
		case hdap.RRSOnlineCheckAnswer:
			v, perr = hdap.DecodeRRSCheckAnswer(m.Payload)
		default:
			v, perr = hdap.DecodeRRSRadio(m.Payload)
		}
	case hdap.ServiceLP:
		switch m.Opcode {
		case hdap.LPStandardRequest:
			v, perr = hdap.DecodeLPRequest(m.Payload)
		case hdap.LPStandardAnswer:
			v, perr = hdap.DecodeLPAnswer(m.Payload)
		case hdap.LPEmergencyReport:
			v, perr = hdap.DecodeLPEmergency(m.Payload)
		case hdap.LPConditionReport:
			v, perr = hdap.DecodeLPReport(m.Payload)
		default:
		}
	case hdap.ServiceTMP:
		switch m.Opcode & 0xFF {
		case hdap.Opcode(hdap.TMPPrivateMessage), hdap.Opcode(hdap.TMPGroupMessage):
			t, err := hdap.DecodeTMPMessage(m.Payload)
			if err == nil {
				pb.summary("HDAP text %q", t.TextUTF8(nil))
			}
			v, perr = t, err
		case hdap.Opcode(hdap.TMPPrivateMessageAnswer):
			v, perr = hdap.DecodeTMPAnswer(m.Payload)
		case hdap.Opcode(hdap.TMPGroupMessageAnswer):
			v, perr = hdap.DecodeTMPGroupAnswer(m.Payload)
		default:
		}
	default:
	}
	if v != nil || perr != nil {
		pn := appResult(fmt.Sprintf("%s payload", m.Header.Service), v, perr, m.Payload, buf, off+po)
		if perr == nil {
			pn.Value = fmt.Sprintf("%T", v)
		}
		n.Children = append(n.Children, pn)
	}
	pb.summary("HDAP %s opcode 0x%04X", m.Header.Service, m.Opcode)
	return n
}
