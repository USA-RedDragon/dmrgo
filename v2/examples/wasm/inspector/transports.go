package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	hyt "github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

const (
	protoIPSC   = "ipsc"
	protoDMRD   = "dmrd"
	protoHytera = "hytera"
	protoMMDVM  = "mmdvm"
	protoHDAP   = "hdap"
	protoBurst  = "burst"

	openBridgeEnhanced = "OpenBridge (FreeDMR enhanced)"
	burstNode          = "DMR burst"
)

func (s *session) detect(d datagram) string {
	p := d.data
	if len(p) == 0 {
		return ""
	}
	inPorts := func(lo, hi int) bool {
		return d.dstPort >= lo && d.dstPort <= hi || d.srcPort >= lo && d.srcPort <= hi
	}
	if inPorts(3002, 3009) {
		if _, err := hdap.DecodeMessage(p); err == nil {
			return protoHDAP
		}
	}
	for _, dl := range []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP, dmrd.DialectLocal} {
		if dmrd.Classify(p, dl) != dmrd.CommandUnknown {
			return protoDMRD
		}
	}
	if bytes.HasPrefix(p, []byte("ZZZZ")) || bytes.HasPrefix(p, []byte("P2P")) {
		return protoHytera
	}
	if len(p) == hyt.FrameLen {
		if _, err := hyt.DecodeFrame(p); err == nil {
			return protoHytera
		}
	}
	if p[0] == mmdvm.FrameStart {
		if _, n, st := mmdvm.Next(p); st == mmdvm.NextFrame && n == len(p) {
			return protoMMDVM
		}
	}
	if d.hint != "" {
		return d.hint
	}
	if len(p) == 33 && !ipscRaw(ipsc.Opcode(p[0])) {
		return protoBurst
	}
	if ipscKnown(ipsc.Opcode(p[0])) && len(p) >= 5 {
		return protoIPSC
	}
	if len(p) == 33 {
		return protoBurst
	}
	if m, err := hdap.DecodeMessage(p); err == nil && m.ChecksumOK() {
		return protoHDAP
	}
	if len(p) == 1 {
		return protoHytera
	}
	return ""
}

func ipscRaw(o ipsc.Opcode) bool {
	switch o {
	case ipsc.OpCallConfirmation, ipsc.OpTextMessageAck, ipsc.OpCallMonitorStatus, ipsc.OpCallMonitorRepeat,
		ipsc.OpCallMonitorNack, ipsc.OpXNL, ipsc.OpWakeup, ipsc.OpCollision, ipsc.OpStatus:
		return true
	default:
	}
	return false
}

func ipscKnown(o ipsc.Opcode) bool {
	switch o {
	case ipsc.OpCallConfirmation, ipsc.OpTextMessageAck, ipsc.OpCallMonitorStatus, ipsc.OpCallMonitorRepeat,
		ipsc.OpCallMonitorNack, ipsc.OpXNL, ipsc.OpGroupVoice, ipsc.OpPrivateVoice, ipsc.OpGroupData,
		ipsc.OpPrivateData, ipsc.OpWakeup, ipsc.OpCollision, ipsc.OpStatus:
		return true
	default:
	}
	return o >= ipsc.OpRegisterRequest && o <= ipsc.OpDeregisterReply
}

func (s *session) decode(d datagram) Packet {
	proto := s.proto
	if proto == "" {
		proto = s.detect(d)
	}
	names := map[string]string{
		protoIPSC: "Motorola IPSC", protoDMRD: "DMRD family (dmrd)", protoHytera: "Hytera IPSC",
		protoMMDVM: "MMDVM modem serial", protoHDAP: "Hytera HDAP", protoBurst: burstNode,
	}
	pb := newPacket(names[proto], d.data, d.meta)
	switch proto {
	case protoIPSC:
		s.ipscPacket(pb, d.data)
	case protoDMRD:
		s.dmrdPacket(pb, d.data)
	case protoHytera:
		s.hyteraPacket(pb, d.data)
	case protoMMDVM:
		s.mmdvmPacket(pb, d.data)
	case protoHDAP:
		pb.p.Tree = append(pb.p.Tree, hdapNode(pb, d.data, 0, 0))
	case protoBurst:
		if len(d.data) != 33 {
			pb.errorf("a DMR burst is 33 octets, this is %d", len(d.data))
			break
		}
		pb.p.Tree = append(pb.p.Tree, s.decodeBurst(pb, layer2.RawBurst(d.data), 0, 0, "burst", nil))
	default:
		pb.p.Protocol = "unknown"
		pb.errorf("no supported protocol matches these %d octets", len(d.data))
	}
	if pb.p.Summary == "" && len(pb.p.Errors) > 0 {
		pb.p.Summary = pb.p.Errors[0]
	}
	return *pb.p
}

func (s *session) ipscPacket(pb *packetBuilder, p []byte) {
	op := ipsc.Opcode(p[0])
	root := &Node{Name: "IPSC", Value: op.String(), Ranges: []Range{{Buf: 0, Start: 0, End: len(p) * 8}}}
	pb.p.Tree = append(pb.p.Tree, root)
	add := func(n ...*Node) { root.Children = append(root.Children, n...) }
	add(field("Opcode", op.String(), 0, 1))
	pb.summary("%s", op)
	switch op {
	case ipsc.OpGroupVoice, ipsc.OpPrivateVoice, ipsc.OpGroupData, ipsc.OpPrivateData:
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			root.Err = err.Error()
			pb.errorf("IPSC user packet: %v", err)
			return
		}
		s.ipscUser(pb, root, p, &u)
	case ipsc.OpPeerListReply:
		c, err := ipsc.DecodeControl(p)
		if err != nil {
			root.Err = err.Error()
			pb.errorf("IPSC peer list: %v", err)
			return
		}
		add(field("ID", fmt.Sprint(c.ID), 1, 5), field("Peers", fmt.Sprintf("%d octets", c.Peers), 5, 7))
		l, _ := ipsc.PeerListOf(p)
		for i := 0; i < l.Len(); i++ {
			e := l.Entry(i)
			o := 7 + 11*i
			add(group(fmt.Sprintf("Peer %d", i), fmt.Sprint(e.ID),
				field("ID", fmt.Sprint(e.ID), o, o+4), field("IP", fmt.Sprintf("%d.%d.%d.%d", e.IP[0], e.IP[1], e.IP[2], e.IP[3]), o+4, o+8),
				field("Port", fmt.Sprint(e.Port), o+8, o+10), field("Mode", fmt.Sprintf("0x%02X operational %t", uint8(e.Mode), e.Mode.Operational()), o+10, o+11)))
		}
	case ipsc.OpRegisterRequest, ipsc.OpRegisterReply, ipsc.OpPeerListRequest, ipsc.OpPeerRegisterRequest, ipsc.OpPeerRegisterReply,
		ipsc.OpAliveRequest, ipsc.OpAliveReply, ipsc.OpPeerAliveRequest, ipsc.OpPeerAliveReply, ipsc.OpDeregisterRequest, ipsc.OpDeregisterReply:
		c, err := ipsc.DecodeControl(p)
		if err != nil {
			root.Err = err.Error()
			pb.errorf("IPSC control: %v", err)
			return
		}
		add(field("ID", fmt.Sprint(c.ID), 1, 5))
		pb.summary("ID %d", c.ID)
		mode := func(o int) {
			add(group("Mode", fmt.Sprintf("0x%02X", uint8(c.Mode)),
				&Node{Name: "Operational", Value: fmt.Sprint(c.Mode.Operational()), Ranges: []Range{{0, o*8 + 1, o*8 + 2}}},
				&Node{Name: "Digital", Value: fmt.Sprint(c.Mode.Digital()), Ranges: []Range{{0, o*8 + 2, o*8 + 4}}},
				&Node{Name: "TS1 link", Value: fmt.Sprint(c.Mode.TS1()), Ranges: []Range{{0, o*8 + 4, o*8 + 6}}},
				&Node{Name: "TS2 link", Value: fmt.Sprint(c.Mode.TS2()), Ranges: []Range{{0, o*8 + 6, o*8 + 8}}}))
			root.Children[len(root.Children)-1].Ranges = []Range{{0, o * 8, o*8 + 8}}
			fl := field("Flags", hex.EncodeToString(c.Flags[:]), o+1, o+5)
			for _, f := range []struct {
				n string
				f ipsc.Flag
			}{{"CSBK", ipsc.FlagCSBK}, {"Repeater monitor", ipsc.FlagRepeaterMonitor}, {"Console app", ipsc.FlagConsoleApp},
				{"XNL status", ipsc.FlagXNLStatus}, {"XNL master", ipsc.FlagXNLMaster}, {"XNL slave", ipsc.FlagXNLSlave},
				{"Authenticated", ipsc.FlagAuth}, {"Data", ipsc.FlagData}, {"Voice", ipsc.FlagVoice}, {"Master peer", ipsc.FlagMasterPeer}} {
				bit := (o+1+int(f.f>>4))*8 + 7 - int(f.f&0x0F)
				fl.Children = append(fl.Children, &Node{Name: f.n, Value: fmt.Sprint(c.Flags.Has(f.f)), Ranges: []Range{{0, bit, bit + 1}}})
			}
			add(fl)
		}
		switch len(p) {
		case 9:
			add(field("Version", hex.EncodeToString(c.Version[:]), 5, 9))
		case 10:
			mode(5)
		case 14:
			mode(5)
			add(field("Version", hex.EncodeToString(c.Version[:]), 10, 14))
		case 16:
			mode(5)
			add(field("Peers", fmt.Sprint(c.Peers), 10, 12), field("Version", hex.EncodeToString(c.Version[:]), 12, 16))
		}
	default:
		r, err := ipsc.DecodeRaw(p)
		if err != nil {
			root.Err = err.Error()
			pb.errorf("IPSC: %v", err)
			return
		}
		add(field("ID", fmt.Sprint(r.ID), 1, 5), field("Body", hex.EncodeToString(r.Body), 5, len(p)))
	}
}

func (s *session) ipscUser(pb *packetBuilder, root *Node, p []byte, u *ipsc.User) {
	add := func(n ...*Node) { root.Children = append(root.Children, n...) }
	add(field("ID", fmt.Sprint(u.ID), 1, 5), field("CallSeq", fmt.Sprint(u.CallSeq), 5, 6),
		field("Source", fmt.Sprint(u.Source), 6, 9), field("Destination", fmt.Sprint(u.Destination), 9, 12),
		field("Priority", fmt.Sprint(u.Priority), 12, 13), field("CallControl", fmt.Sprintf("0x%08X", u.CallControl), 13, 17),
		group("Info", fmt.Sprintf("0x%02X", uint8(u.Info)),
			&Node{Name: "Privacy", Value: fmt.Sprint(u.Info.Privacy()), Ranges: []Range{{0, 136, 137}}},
			&Node{Name: "End", Value: fmt.Sprint(u.Info.End()), Ranges: []Range{{0, 137, 138}}},
			&Node{Name: "Slot", Value: u.Info.Slot().String(), Ranges: []Range{{0, 138, 139}}}),
		group("RTP", "",
			field("Flags", fmt.Sprintf("0x%02X", u.RTP.Flags), 18, 19),
			&Node{Name: "Marker", Value: fmt.Sprint(u.RTP.Marker), Ranges: []Range{{0, 152, 153}}},
			&Node{Name: "PayloadType", Value: fmt.Sprintf("0x%02X", u.RTP.PayloadType), Ranges: []Range{{0, 153, 160}}},
			field("Seq", fmt.Sprint(u.RTP.Seq), 20, 22), field("Timestamp", fmt.Sprint(u.RTP.Timestamp), 22, 26),
			field("SSRC", fmt.Sprint(u.RTP.SSRC), 26, 30)),
		field("Burst", burstTypeText(u.Burst), 30, 31))
	root.Children[len(root.Children)-3].Ranges = []Range{{0, 136, 144}}
	root.Children[len(root.Children)-2].Ranges = []Range{{0, 144, 240}}
	pb.summary("%d > %d %s", u.Source, u.Destination, u.Info.Slot())
	key := fmt.Sprintf("ipsc/%d/%d", u.ID, u.Info.Slot())
	src := layer2.SyncSourceBS
	if u.IsVoice() {
		v := &u.Voice
		vn := group("Voice", fmt.Sprintf("layout %d", v.Layout),
			field("Length", fmt.Sprint(v.Length), 31, 32), field("Control", fmt.Sprintf("0x%02X", v.Control), 32, 33),
			field("AMBE", hex.EncodeToString(v.AMBE[:]), 33, 52))
		switch v.Layout {
		case ipsc.VoiceLayoutBCDF:
			vn.Children = append(vn.Children, field("Embedded", hex.EncodeToString(v.Embedded[:]), 52, 56), field("EMB", embInfoText(v.EMB), 56, 57))
		case ipsc.VoiceLayoutE:
			vn.Children = append(vn.Children, field("Embedded", hex.EncodeToString(v.Embedded[:]), 52, 56), field("LC", hex.EncodeToString(v.LC[:]), 56, 65), field("EMB", embInfoText(v.EMB), 65, 66))
			vn.Children = append(vn.Children, s.lcNode(pb, "Full LC of burst E", v.LC, 56))
		default:
		}
		vn.Ranges = []Range{{0, 31 * 8, len(p) * 8}}
		add(vn)
	} else {
		d := &u.Data
		dn := group("Data", fmt.Sprintf("%d bits", d.Bits),
			field("Status", fmt.Sprintf("0x%02X", d.Status), 31, 32), field("Words", fmt.Sprint(d.Words), 32, 34))
		if d.HasBody {
			n := int(d.Bits+7) / 8
			dn.Children = append(dn.Children, field("Flags", hex.EncodeToString(d.Flags[:]), 34, 36), field("Bits", fmt.Sprint(d.Bits), 36, 38),
				field("Payload", hex.EncodeToString(d.Octets[:n]), 38, 38+n))
			tn := field("Trailer", hex.EncodeToString(d.Trailer[:d.TrailerLen]), 38+n, 38+n+int(d.TrailerLen))
			if st, ok := d.SlotType(); ok {
				tn.Children = append(tn.Children, &Node{Name: "Slot type", Value: fmt.Sprintf("0x%02X", uint8(st))})
			}
			if r, ok := d.RSSI(); ok {
				tn.Children = append(tn.Children, field("RSSI", fmt.Sprint(r), len(p)-2, len(p)))
			}
			dn.Children = append(dn.Children, tn)
			s.payloadBase = 38
		}
		dn.Ranges = []Range{{0, 31 * 8, len(p) * 8}}
		add(dn)
	}
	raw, err := u.RawBurst(1, src)
	if err != nil {
		add(&Node{Name: "DMR burst", Value: "not rebuilt", Err: err.Error()})
		s.payloadBase = 0
		return
	}
	bb := pb.buffer("burst rebuilt from the IPSC payload", raw[:], -1, nil)
	add(s.decodeBurst(pb, raw, bb, 0, key, hintOptions(u.Hint(0))))
	s.payloadBase = 0
}

func burstTypeText(b ipsc.BurstType) string {
	switch b {
	case ipsc.BurstVoice, ipsc.BurstVoice | 0x80:
		return fmt.Sprintf("voice (0x%02X)", uint8(b))
	case ipsc.BurstSyncLost, ipsc.BurstSyncLost | 0x80:
		return fmt.Sprintf("sync lost (0x%02X)", uint8(b))
	case ipsc.BurstRaw:
		return "raw bits (0x1F)"
	}
	if b <= 8 {
		return fmt.Sprintf("%s (0x%02X)", enums.DataType(b), uint8(b))
	}
	return fmt.Sprintf("0x%02X", uint8(b))
}

func (s *session) dmrdDialect(p []byte) (dmrd.Dialect, dmrd.Command) {
	order := []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP, dmrd.DialectLocal}
	if s.dialect != dmrd.DialectUnknown {
		order = append([]dmrd.Dialect{s.dialect}, order...)
	}
	var first dmrd.Command
	var firstD dmrd.Dialect
	for _, dl := range order {
		c := dmrd.Classify(p, dl)
		if c == dmrd.CommandUnknown {
			continue
		}
		if first == dmrd.CommandUnknown {
			first, firstD = c, dl
		}
		if _, err := dmrd.DecodeControl(p, dl); err == nil {
			return dl, c
		}
	}
	return firstD, first
}

func dialectName(d dmrd.Dialect) string {
	switch d {
	case dmrd.DialectMMDVM:
		return "MMDVM network protocol"
	case dmrd.DialectHBRP:
		return "HomeBrew Repeater Protocol"
	case dmrd.DialectLocal:
		return "MMDVMHost to DMRGateway local link"
	default:
	}
	return d.String()
}

type dmrdCtx struct {
	pb   *packetBuilder
	root *Node
	p    []byte
}

func (c *dmrdCtx) add(n ...*Node) { c.root.Children = append(c.root.Children, n...) }

func (c *dmrdCtx) fail(what string, err error) {
	c.root.Err = err.Error()
	c.pb.errorf("%s: %v", what, err)
}

func (s *session) dmrdPacket(pb *packetBuilder, p []byte) {
	dl, cmd := s.dmrdDialect(p)
	c := &dmrdCtx{pb: pb, root: &Node{Name: cmd.String(), Ranges: []Range{{0, 0, len(p) * 8}}}, p: p}
	pb.p.Tree = append(pb.p.Tree, c.root)
	tagLen := len(cmd.String())
	c.add(field("Tag", fmt.Sprintf("%q", p[:min(tagLen, len(p))]), 0, tagLen))
	pb.summary("%s", cmd)
	switch cmd {
	case dmrd.CommandDMRD, dmrd.CommandDMRE, dmrd.CommandBCKA, dmrd.CommandBCSQ, dmrd.CommandBCST, dmrd.CommandBCVE:
		s.dmrdBridge(c, cmd)
	case dmrd.CommandDMRA, dmrd.CommandDMRG:
		dmrdAliasPosition(c, cmd)
	case dmrd.CommandRPTC:
		s.dmrdConfig(c)
	default:
		s.dmrdControl(c, dl, cmd, tagLen)
	}
}

func (s *session) dmrdBridge(c *dmrdCtx, cmd dmrd.Command) {
	p, pb := c.p, c.pb
	switch {
	case cmd == dmrd.CommandDMRD && len(p) == 73:
		pb.p.Protocol = "OpenBridge"
		b, err := dmrd.DecodeBridge(p)
		if err != nil {
			c.fail("OpenBridge", err)
			return
		}
		c.root.Value = "OpenBridge version 1"
		s.dmrdData(pb, c.root, &b.Data)
		c.add(field("Signature", hex.EncodeToString(b.Signature[:]), 53, 73))
	case cmd == dmrd.CommandDMRD:
		pb.p.Protocol = "DMRD (same in MMDVM network protocol, HBRP and local link)"
		d, err := dmrd.DecodeData(p)
		if err != nil {
			c.fail("DMRD", err)
			return
		}
		s.dmrdData(pb, c.root, &d)
	case cmd == dmrd.CommandDMRE:
		pb.p.Protocol = openBridgeEnhanced
		b, err := dmrd.DecodeBridge(p)
		if err != nil {
			c.fail("OpenBridge", err)
			return
		}
		c.root.Value = fmt.Sprintf("OpenBridge version %d", b.Version)
		s.dmrdData(pb, c.root, &b.Data)
		c.add(field("Version", fmt.Sprint(b.Version), 55, 56), field("Time", fmt.Sprint(b.Time), 56, 64), field("SourceServer", fmt.Sprint(b.SourceServer), 64, 68))
		o := 68
		if b.Version == 5 {
			c.add(field("SourceRepeater", fmt.Sprint(b.SourceRepeater), 68, 72))
			o = 72
		}
		c.add(field("Hops", fmt.Sprint(b.Hops), o, o+1), field("Hash", hex.EncodeToString(b.Signature[:16]), o+1, o+17))
	default:
		pb.p.Protocol = openBridgeEnhanced
		bc, err := dmrd.DecodeBridgeControl(p)
		if err != nil {
			c.fail("OpenBridge control", err)
			return
		}
		if cmd == dmrd.CommandBCSQ {
			c.add(field("TalkGroup", fmt.Sprint(bc.TalkGroup), 4, 7), field("Stream", fmt.Sprintf("0x%08X", bc.Stream), 7, 11))
		}
		if cmd == dmrd.CommandBCVE {
			c.add(field("Version", fmt.Sprint(bc.Version), 4, 5))
		}
		c.add(field("Signature", hex.EncodeToString(bc.Signature[:]), len(p)-20, len(p)))
	}
}

func dmrdAliasPosition(c *dmrdCtx, cmd dmrd.Command) {
	p := c.p
	local := cmd == dmrd.CommandDMRA && len(p) == 15 || cmd == dmrd.CommandDMRG && len(p) == 14
	c.pb.p.Protocol = dialectName(dmrd.DialectMMDVM)
	if local {
		c.pb.p.Protocol = dialectName(dmrd.DialectLocal)
	}
	var v any
	var err error
	switch {
	case cmd == dmrd.CommandDMRA && local:
		v, err = dmrd.DecodeLocalTalkerAlias(p)
	case cmd == dmrd.CommandDMRA:
		v, err = dmrd.DecodeTalkerAlias(p)
	case local:
		v, err = dmrd.DecodeLocalPosition(p)
	default:
		v, err = dmrd.DecodePosition(p)
	}
	if err != nil {
		c.fail(cmd.String(), err)
		return
	}
	c.add(walkBits(reflect.ValueOf(v), 0, 0)...)
}

func (s *session) dmrdConfig(c *dmrdCtx) {
	p := c.p
	if len(p) == 306 {
		c.pb.p.Protocol = dialectName(dmrd.DialectHBRP)
		s.dialect = dmrd.DialectHBRP
		h, err := dmrd.HBRPConfigOf(p)
		if err != nil {
			c.fail("RPTC", err)
			return
		}
		id, ierr := h.ID()
		c.add(numField("ID", id, ierr, 12, 20), textField("Callsign", h.Callsign(), 4, 12))
		configFields(c.add, h.RXFrequency, h.TXFrequency, h.Power, h.ColorCode, h.Height, 20)
		c.add(textField("Latitude", h.Latitude(), 42, 50), textField("Longitude", h.Longitude(), 50, 59),
			textField("Location", h.Location(), 62, 82), textField("Description", h.Description(), 82, 102),
			textField("URL", h.URL(), 102, 226), textField("SoftwareID", h.SoftwareID(), 226, 266), textField("PackageID", h.PackageID(), 266, 306))
		return
	}
	c.pb.p.Protocol = dialectName(dmrd.DialectMMDVM)
	m, err := dmrd.MMDVMConfigOf(p)
	if err != nil {
		c.fail("RPTC", err)
		return
	}
	s.dialect = dmrd.DialectMMDVM
	c.add(field("ID", fmt.Sprint(m.ID()), 4, 8), textField("Callsign", m.Callsign(), 8, 16))
	configFields(c.add, m.RXFrequency, m.TXFrequency, m.Power, m.ColorCode, m.Height, 16)
	sl, serr := m.Slots()
	c.add(textField("Latitude", m.Latitude(), 38, 46), textField("Longitude", m.Longitude(), 46, 55),
		textField("Location", m.Location(), 58, 78), textField("Description", m.Description(), 78, 97), numField("Slots", uint32(sl), serr, 97, 98),
		textField("URL", m.URL(), 98, 222), textField("SoftwareID", m.SoftwareID(), 222, 262), textField("PackageID", m.PackageID(), 262, 302))
}

func (s *session) dmrdControl(c *dmrdCtx, dl dmrd.Dialect, cmd dmrd.Command, tagLen int) {
	p := c.p
	c.pb.p.Protocol = dialectName(dl)
	m, err := dmrd.DecodeControl(p, dl)
	if err != nil {
		c.fail(cmd.String(), err)
		return
	}
	if dl == dmrd.DialectHBRP || dl == dmrd.DialectMMDVM {
		s.dialect = dl
	}
	c.root.Value = dialectName(dl)
	idLen := 4
	if dl == dmrd.DialectHBRP {
		idLen = 8
	}
	o := tagLen
	textOff := len(p) - len(m.Text)
	if o+idLen <= textOff {
		c.add(field("ID", fmt.Sprint(m.ID), o, o+idLen))
		o += idLen
	}
	if m.HasSalt {
		c.add(field("Salt", fmt.Sprintf("0x%08X", m.Salt), o, o+idLen))
		o += idLen
	}
	if cmd == dmrd.CommandRPTK {
		c.add(field("Token", hex.EncodeToString(m.Token[:]), o, o+idLen*8))
	}
	if len(m.Text) == 0 {
		return
	}
	c.add(field("Text", fmt.Sprintf("%q", m.Text), textOff, len(p)))
	if cmd == dmrd.CommandRPTO {
		if opts, err := dmrd.ParseDMRPlusOptions(m.Text); err == nil {
			c.add(&Node{Name: "DMR+ options", Children: walkPlain(reflect.ValueOf(opts))})
		}
	}
}

func numField(name string, v uint32, err error, a, b int) *Node {
	n := field(name, fmt.Sprint(v), a, b)
	if err != nil {
		n.Err = err.Error()
	}
	return n
}

func textField(name string, v []byte, a, b int) *Node {
	return field(name, fmt.Sprintf("%q", v), a, b)
}

func configFields(add func(...*Node), rx, tx func() (uint32, error), pw func() (uint8, error), cc func() (enums.ColorCode, error), h func() (uint16, error), o int) {
	r, rerr := rx()
	t, terr := tx()
	p, perr := pw()
	c, cerr := cc()
	ht, herr := h()
	add(numField("RXFrequency", r, rerr, o, o+9), numField("TXFrequency", t, terr, o+9, o+18), numField("Power", uint32(p), perr, o+18, o+20),
		numField("ColorCode", uint32(c), cerr, o+20, o+22))
	hn := numField("Height", uint32(ht), herr, o+39, o+42)
	add(hn)
}

func (s *session) dmrdData(pb *packetBuilder, root *Node, d *dmrd.Data) {
	ns := walkBits(reflect.ValueOf(d), 0, 0)
	root.Children = append(root.Children, ns...)
	pb.summary("%d > %d %s seq %d", d.Source, d.Destination, d.Slot, d.Seq)
	opts := hintOptions(d.Hint(0))
	bn := findNode(root.Children, "Burst")
	key := fmt.Sprintf("dmrd/%d/%d", d.Repeater, d.Slot)
	if bn != nil {
		bn.Children = append(bn.Children, s.decodeBurst(pb, d.Burst, 0, 160, key, opts))
	}
}

func hyteraPairs() []int {
	m := make([]int, 33)
	for i := range m {
		m[i] = 26 + (i ^ 1)
	}
	return m
}

func (s *session) hyteraPacket(pb *packetBuilder, p []byte) {
	root := &Node{Name: "Hytera IPSC", Ranges: []Range{{0, 0, len(p) * 8}}}
	pb.p.Tree = append(pb.p.Tree, root)
	add := func(n ...*Node) { root.Children = append(root.Children, n...) }
	switch {
	case len(p) == 1 || bytes.HasPrefix(p, []byte("ZZZZ")):
		h, err := hyt.DecodeHeartbeat(p)
		root.Value = "heartbeat"
		if err != nil {
			root.Err = err.Error()
			pb.errorf("heartbeat: %v", err)
			return
		}
		if h.Keepalive {
			root.Value = "keepalive"
			add(field("Code", fmt.Sprintf("0x%02X", h.Code), 0, 1))
			pb.summary("keepalive")
			return
		}
		pb.summary("heartbeat")
		add(field("Signature", `"ZZZZ"`, 0, 4), field("Code", fmt.Sprintf("0x%02X", h.Code), 4, 5), field("Seq", fmt.Sprintf("0x%02X", h.Seq), 12, 13),
			field("Status", hex.EncodeToString(h.Status[:]), 13, 16))
	case bytes.HasPrefix(p, []byte("P2P")):
		c, err := hyt.DecodeP2P(p)
		root.Value = "P2P"
		if err != nil {
			root.Err = err.Error()
			pb.errorf("P2P: %v", err)
			return
		}
		add(field("Signature", `"P2P"`, 0, 3), field("Op", fmt.Sprintf("0x%02X", c.Op), 3, 4), field("Code", fmt.Sprintf("0x%02X", c.Code), 4, 5),
			field("Seq", fmt.Sprint(c.Seq), 12, 13), field("Status", hex.EncodeToString(c.Status[:]), 13, 16))
		if len(c.Body) > 0 {
			bn := field("Body", hex.EncodeToString(c.Body), 20, len(p))
			if sv, ok := c.Service(); ok {
				bn.Children = append(bn.Children, field("Service", sv.String(), 20, 21))
			}
			if port, ok := c.RedirectPort(); ok {
				bn.Children = append(bn.Children, field("RedirectPort", fmt.Sprint(port), len(p)-2, len(p)))
			}
			add(bn)
		}
		pb.summary("P2P op 0x%02X code 0x%02X", c.Op, c.Code)
	default:
		f, err := hyt.DecodeFrame(p)
		root.Value = "DMR frame"
		if err != nil {
			root.Err = err.Error()
			pb.errorf("DMR frame: %v", err)
			return
		}
		add(field("Port", fmt.Sprint(f.Port), 0, 2), field("Mark", fmt.Sprintf("0x%04X", f.Mark), 2, 4), field("Seq", fmt.Sprint(f.Seq), 4, 5),
			field("PacketType", fmt.Sprintf("0x%02X", f.PacketType), 8, 9), field("Slot", f.Slot.String(), 16, 18),
			field("SlotType", f.SlotType.String(), 18, 20), field("ColorCode", fmt.Sprint(f.ColorCode), 20, 22),
			field("FrameType", fmt.Sprintf("0x%04X", f.FrameType), 22, 24))
		bn := &Node{Name: "Burst", Value: "octet pairs swapped", Ranges: []Range{{0, 26 * 8, 58 * 8}, {0, 59 * 8, 60 * 8}}}
		add(bn, field("Pad", fmt.Sprintf("0x%02X", f.Pad), 58, 59), field("CallType", fmt.Sprintf("0x%02X", f.CallType), 62, 63),
			field("Destination", fmt.Sprint(f.Destination), 64, 67), field("Source", fmt.Sprint(f.Source), 68, 71))
		pb.summary("%d > %d %s %s", f.Source, f.Destination, f.Slot, f.SlotType)
		if !f.HasBurst() {
			bn.Value = "no burst in a " + f.SlotType.String() + " packet"
			return
		}
		bb := pb.buffer("burst (octet pairs swapped back)", f.Burst[:], 0, hyteraPairs())
		opts := hintOptions(f.Hint(0))
		bn.Children = append(bn.Children, s.decodeBurst(pb, f.Burst, bb, 0, fmt.Sprintf("hytera/%d", f.Slot), opts))
	}
}

func (s *session) mmdvmPacket(pb *packetBuilder, p []byte) {
	f, n, st := mmdvm.Next(p)
	root := &Node{Name: "MMDVM frame", Ranges: []Range{{0, 0, len(p) * 8}}}
	pb.p.Tree = append(pb.p.Tree, root)
	if st != mmdvm.NextFrame || n != len(p) {
		root.Err = fmt.Sprintf("not one whole frame (status %d, length %d of %d)", st, n, len(p))
		pb.errorf("MMDVM: %s", root.Err)
		return
	}
	po := len(p) - len(f.Payload)
	add := func(n ...*Node) { root.Children = append(root.Children, n...) }
	add(field("Start", "0xE0", 0, 1), field("Length", fmt.Sprint(len(p)), 1, po-1), field("Command", f.Command.String(), po-1, po))
	root.Value = f.Command.String()
	pb.summary("%s", f.Command)
	switch f.Command {
	case mmdvm.CommandDMRData1, mmdvm.CommandDMRData2, mmdvm.CommandDMRLost1, mmdvm.CommandDMRLost2:
		s.mmdvmDMR(pb, root, f, po)
		return
	case mmdvm.CommandDMRAloha:
		if a, err := mmdvm.DecodeDMRAloha(f.Payload); err == nil {
			add(s.decodeBurst(pb, a.Burst, 0, po*8, "mmdvm/aloha", nil))
			return
		}
	default:
	}
	v, err := mmdvmTyped(f)
	if errors.Is(err, errNoMessage) {
		v, err = nil, nil
	}
	if err != nil {
		root.Err = err.Error()
		pb.errorf("%s: %v", f.Command, err)
		return
	}
	if v == nil {
		if len(f.Payload) > 0 {
			add(field("Payload", hex.EncodeToString(f.Payload), po, len(p)))
		}
		return
	}
	pn := &Node{Name: "Payload", Value: fmt.Sprintf("%T", v), Ranges: []Range{{0, po * 8, len(p) * 8}}}
	rv := reflect.ValueOf(v)
	pv := reflect.New(rv.Type())
	pv.Elem().Set(rv)
	if usesBits(rv.Type()) {
		pn.Children = walkBits(pv, 0, po*8)
	} else {
		pn.Children = walkPlain(pv)
		for _, c := range pn.Children {
			if c.Name == "Description" {
				if d, ok := slicePos(pv.Elem().FieldByName("Description"), p); ok {
					c.Ranges = []Range{{0, d * 8, (d + pv.Elem().FieldByName("Description").Len()) * 8}}
				}
			}
		}
	}
	add(pn)
}

func embInfoText(e pdu.EMBInfo) string {
	return fmt.Sprintf("0x%02X: CC %d, PI %t, LCSS %s", uint8(e), e.ColorCode(), e.PI(), e.LCSS())
}

func hintOptions(h layer2.Hint) *layer2.DecodeOptions {
	o := &layer2.DecodeOptions{Kind: h.Kind, VoiceBurst: h.VoiceBurst}
	if h.HasDataType && h.Kind == layer2.KindData {
		dt := h.DataType
		o.DataType = &dt
	}
	return o
}

func (s *session) mmdvmDMR(pb *packetBuilder, root *Node, f mmdvm.Frame, po int) {
	add := func(n ...*Node) { root.Children = append(root.Children, n...) }
	d, err := mmdvm.DecodeDMRData(f)
	if err != nil {
		root.Err = err.Error()
		pb.errorf("DMR data: %v", err)
		return
	}
	add(&Node{Name: "Slot", Value: d.Slot.String()})
	if d.Lost {
		add(&Node{Name: "Lost", Value: "true"})
		return
	}
	c := d.Control
	cn := group("Control", fmt.Sprintf("0x%02X", uint8(c)),
		&Node{Name: "Idle", Value: fmt.Sprint(c.Idle()), Ranges: []Range{{0, po * 8, po*8 + 1}}},
		&Node{Name: "DataSync", Value: fmt.Sprint(c.DataSync()), Ranges: []Range{{0, po*8 + 1, po*8 + 2}}},
		&Node{Name: "VoiceSync", Value: fmt.Sprint(c.VoiceSync()), Ranges: []Range{{0, po*8 + 2, po*8 + 3}}},
		&Node{Name: "Low nibble", Value: fmt.Sprint(c.N()), Ranges: []Range{{0, po*8 + 4, po*8 + 8}}})
	cn.Ranges = []Range{{0, po * 8, po*8 + 8}}
	bn := field("Burst", hex.EncodeToString(d.Burst[:]), po+1, po+34)
	add(cn, bn)
	if d.HasRSSI {
		add(field("RSSI", fmt.Sprint(d.RSSI), po+34, po+36))
	}
	pb.summary("%s", d.Slot)
	var opts *layer2.DecodeOptions
	if c.VoiceSync() {
		opts = &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA}
	} else if !c.DataSync() && c.N() >= 1 && c.N() <= 5 {
		opts = &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA + enums.VoiceBurstType(c.N())}
	}
	bn.Children = append(bn.Children, s.decodeBurst(pb, d.Burst, 0, (po+1)*8, fmt.Sprintf("mmdvm/%d", d.Slot), opts))
}

func mmdvmTyped(f mmdvm.Frame) (any, error) {
	switch f.Command {
	case mmdvm.CommandDebug1, mmdvm.CommandDebug2, mmdvm.CommandDebug3, mmdvm.CommandDebug4, mmdvm.CommandDebug5:
		return mmdvm.DecodeDebug(f)
	case mmdvm.CommandGetVersion:
		if len(f.Payload) > 0 {
			return mmdvm.DecodeVersion(f.Payload)
		}
	case mmdvm.CommandGetStatus:
		if len(f.Payload) > 0 {
			if v, err := mmdvm.DecodeStatusV2(f.Payload); err == nil {
				return v, nil
			}
			return mmdvm.DecodeStatusV1(f.Payload)
		}
	case mmdvm.CommandSetConfig:
		if len(f.Payload) == 23 {
			return mmdvm.DecodeConfigV1(f.Payload)
		}
		return mmdvm.DecodeConfigV2(f.Payload)
	case mmdvm.CommandSetFreq:
		return mmdvm.DecodeSetFreq(f.Payload)
	case mmdvm.CommandSetMode:
		return mmdvm.DecodeSetMode(f.Payload)
	case mmdvm.CommandAck:
		return mmdvm.DecodeAck(f.Payload)
	case mmdvm.CommandNak:
		return mmdvm.DecodeNak(f.Payload)
	case mmdvm.CommandDMRStart:
		return mmdvm.DecodeDMRStart(f.Payload)
	case mmdvm.CommandDMRAbort:
		return mmdvm.DecodeDMRAbort(f.Payload)
	case mmdvm.CommandDMRShortLC:
		return mmdvm.DecodeDMRShortLC(f.Payload)
	case mmdvm.CommandDMRAloha:
		return mmdvm.DecodeDMRAloha(f.Payload)
	default:
	}
	return nil, errNoMessage
}

var errNoMessage = errors.New("no typed message for this command")
