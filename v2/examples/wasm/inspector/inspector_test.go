package main

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func repo(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func inspect(t *testing.T, in []byte, opt Options) Result {
	t.Helper()
	r := Inspect(in, opt)
	checkRanges(t, r)
	return r
}

func checkRanges(t *testing.T, r Result) {
	t.Helper()
	for _, p := range r.Packets {
		var walk func(ns []*Node)
		walk = func(ns []*Node) {
			for _, n := range ns {
				for _, g := range n.Ranges {
					if g.Buf < 0 || g.Buf >= len(p.Buffers) || g.Start < 0 || g.End <= g.Start || g.End > len(p.Buffers[g.Buf].Hex)*4 {
						t.Errorf("packet %d: field %s has range %+v outside its buffer", p.Index, n.Name, g)
					}
				}
				walk(n.Children)
			}
		}
		walk(p.Tree)
		for bi, b := range p.Buffers {
			for _, o := range b.Map {
				if b.Parent < 0 || b.Parent >= len(p.Buffers) || o < 0 || o >= len(p.Buffers[b.Parent].Hex)/2 {
					t.Errorf("packet %d: buffer %d maps to octet %d outside buffer %d", p.Index, bi, o, b.Parent)
				}
			}
		}
	}
}

func node(t *testing.T, p Packet, path string) *Node {
	t.Helper()
	n := findNode(p.Tree, path)
	if n == nil {
		t.Fatalf("packet %d (%s): no field %s", p.Index, p.Summary, path)
	}
	return n
}

func wantValue(t *testing.T, p Packet, path, want string) *Node {
	t.Helper()
	n := node(t, p, path)
	if !strings.HasPrefix(n.Value, want) {
		t.Errorf("packet %d: %s = %q, want %q", p.Index, path, n.Value, want)
	}
	return n
}

func wantOctets(t *testing.T, n *Node, start, end int) {
	t.Helper()
	if len(n.Ranges) != 1 || n.Ranges[0] != (Range{Buf: 0, Start: start * 8, End: end * 8}) {
		t.Errorf("%s: ranges %+v, want packet octets %d to %d", n.Name, n.Ranges, start, end)
	}
}

func noErrors(t *testing.T, r Result) {
	t.Helper()
	for _, p := range r.Packets {
		if len(p.Errors) > 0 {
			t.Errorf("packet %d (%s): errors %v", p.Index, p.Summary, p.Errors)
		}
	}
}

func protocols(t *testing.T, r Result, want string, n int) {
	t.Helper()
	if len(r.Packets) != n {
		t.Fatalf("%d packets, want %d", len(r.Packets), n)
	}
	for _, p := range r.Packets {
		if !strings.Contains(p.Protocol, want) {
			t.Errorf("packet %d: protocol %q, want %q", p.Index, p.Protocol, want)
		}
	}
}

func TestIPSC(t *testing.T) {
	t.Parallel()
	r := inspect(t, repo(t, "transport/ipsc/testdata/xpr8400.txt"), Options{})
	protocols(t, r, "Motorola IPSC", 182)
	noErrors(t, Result{Packets: r.Packets[121:]})
	hdr := r.Packets[121]
	wantOctets(t, wantValue(t, hdr, "IPSC.Source", "3191868"), 6, 9)
	wantValue(t, hdr, "IPSC.Destination", "9")
	wantValue(t, hdr, "IPSC.Info.Slot", "TS2")
	wantValue(t, hdr, "IPSC.Burst", "Voice LC Header")
	sa := wantValue(t, hdr, "IPSC.DMR burst.FullLinkControl.GroupVoice.SourceAddress", "3191868")
	b := hdr.Buffers[sa.Ranges[0].Buf]
	if b.Parent != 0 || b.Map[0] != 38 {
		t.Errorf("payload buffer maps to buffer %d octet %v, want packet octet 38", b.Parent, b.Map)
	}
	wantValue(t, hdr, "IPSC.DMR burst.Slot type.ColorCode", "7")
	e := r.Packets[126]
	wantValue(t, e, "IPSC.Voice.Full LC of burst E.GroupVoice.GroupAddress", "9")
	wantValue(t, e, "IPSC.DMR burst.Embedded LC (assembled).GroupVoice.SourceAddress", "3191868")
	wantValue(t, e, "IPSC.DMR burst.Voice position", "E")
	sms := r.Packets[181]
	wantValue(t, sms, "IPSC.DMR burst.Data packet (reassembled).Text message", `".ADGJMPTW`)
	wantValue(t, r.Packets[117], "IPSC.Opcode", "AliveReply")
}

func TestDMRDMMDVM(t *testing.T) {
	t.Parallel()
	r := inspect(t, repo(t, "transport/dmrd/testdata/dmrgateway-master.txt"), Options{})
	if len(r.Packets) != 29 {
		t.Fatalf("%d packets", len(r.Packets))
	}
	noErrors(t, r)
	for _, p := range r.Packets {
		if strings.Contains(p.Protocol, "HomeBrew") {
			t.Errorf("packet %d: %s in an MMDVM network protocol capture", p.Index, p.Protocol)
		}
	}
	login := r.Packets[0]
	if login.Protocol != "MMDVM network protocol" {
		t.Errorf("RPTL protocol %q", login.Protocol)
	}
	wantOctets(t, wantValue(t, login, "RPTL.ID", "3149999"), 4, 8)
	cfg := r.Packets[4]
	wantOctets(t, wantValue(t, cfg, "RPTC.Callsign", `"KI5VMF"`), 8, 16)
	wantValue(t, cfg, "RPTC.ColorCode", "9")
	wantValue(t, cfg, "RPTC.Location", `"Anytown"`)
	d := r.Packets[8]
	wantOctets(t, wantValue(t, d, "DMRD.Source", "3191868"), 5, 8)
	wantValue(t, d, "DMRD.Burst.DMR burst.FullLinkControl.GroupVoice.GroupAddress", "91")
	sync := node(t, d, "DMRD.Burst.DMR burst.SYNC")
	if sync.Ranges[0] != (Range{Buf: 0, Start: 160 + 108, End: 160 + 156}) {
		t.Errorf("SYNC range %+v", sync.Ranges)
	}
	if findNode(d.Tree, "DMRD.UUID") != nil {
		t.Error("UUID shown on a 55-octet DMRD")
	}
	wantValue(t, r.Packets[13], "DMRD.Burst.DMR burst.Embedded LC (assembled).GroupVoice.SourceAddress", "3191868")
	wantValue(t, r.Packets[6], "RPTO.DMR+ options.StartRef.Value", "4013")
}

func TestDMRDLocalAndOpenBridge(t *testing.T) {
	t.Parallel()
	r := inspect(t, repo(t, "transport/dmrd/testdata/mmdvmhost-dmrgateway.txt"), Options{})
	noErrors(t, r)
	if r.Packets[0].Protocol != "MMDVMHost to DMRGateway local link" {
		t.Errorf("DMRP protocol %q", r.Packets[0].Protocol)
	}
	wantValue(t, r.Packets[2], "DMRD.Burst.DMR burst.FullLinkControl.GroupVoice.SourceAddress", "3191868")

	ob := inspect(t, repo(t, "transport/dmrd/testdata/hblink3-openbridge.txt"), Options{})
	protocols(t, ob, "OpenBridge", 8)
	noErrors(t, ob)
	wantOctets(t, node(t, ob.Packets[0], "DMRD.Signature"), 53, 73)
	wantValue(t, ob.Packets[0], "DMRD.Burst.DMR burst.FullLinkControl.FLCO", "Group Voice Channel User")

	d, err := dmrd.DecodeData(mustHex(t, string(repo(t, "transport/dmrd/testdata/hblink3-openbridge.txt")[4:4+106])))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []uint8{4, 5} {
		d.Size = 55
		pkt, err := (&dmrd.Bridge{Version: v, Data: d, Time: 1234, SourceServer: 2001, SourceRepeater: 3001, Hops: 2}).AppendBinary(nil)
		if err != nil {
			t.Fatal(err)
		}
		e := inspect(t, []byte(hex.EncodeToString(pkt)), Options{})
		protocols(t, e, "OpenBridge", 1)
		noErrors(t, e)
		wantValue(t, e.Packets[0], "DMRE.Hops", "2")
		wantValue(t, e.Packets[0], "DMRE.SourceServer", "2001")
		if v == 5 {
			wantOctets(t, wantValue(t, e.Packets[0], "DMRE.SourceRepeater", "3001"), 68, 72)
		}
	}
	ka, err := (&dmrd.BridgeControl{Command: dmrd.CommandBCKA}).AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	k := inspect(t, ka, Options{Format: "binary"})
	protocols(t, k, "OpenBridge", 1)
	noErrors(t, k)
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHBRP(t *testing.T) {
	t.Parallel()
	lines := make([]string, 0, 4)
	for _, c := range []dmrd.Control{
		{Command: dmrd.CommandRPTL, ID: 3149999},
		{Command: dmrd.CommandMSTACK, ID: 3149999, Salt: 0x1234ABCD, HasSalt: true},
		{Command: dmrd.CommandMSTPING, ID: 3149999},
	} {
		p, err := c.AppendBinary(nil, dmrd.DialectHBRP)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, hex.EncodeToString(p))
	}
	var cfg dmrd.HBRPConfig
	cfg.Reset()
	cfg.SetID(3149999)
	if err := cfg.SetCallsign([]byte("KI5VMF")); err != nil {
		t.Fatal(err)
	}
	lines = append(lines, hex.EncodeToString(cfg[:]))
	r := inspect(t, []byte(strings.Join(lines, "\n")), Options{})
	protocols(t, r, "HomeBrew Repeater Protocol", 4)
	noErrors(t, r)
	wantOctets(t, wantValue(t, r.Packets[0], "RPTL.ID", "3149999"), 4, 12)
	wantValue(t, r.Packets[1], "MSTACK.Salt", "0x1234ABCD")
	wantOctets(t, wantValue(t, r.Packets[3], "RPTC.ID", "3149999"), 12, 20)
	wantValue(t, r.Packets[3], "RPTC.Callsign", `"KI5VMF"`)
}

func TestHytera(t *testing.T) {
	t.Parallel()
	r := inspect(t, repo(t, "transport/hytera/testdata/built.txt"), Options{})
	protocols(t, r, "Hytera IPSC", 16)
	noErrors(t, r)
	h := r.Packets[0]
	wantOctets(t, wantValue(t, h, "Hytera IPSC.Source", "3191868"), 68, 71)
	wantValue(t, h, "Hytera IPSC.SlotType", "Voice LC Header")
	wantValue(t, h, "Hytera IPSC.Burst.DMR burst.FullLinkControl.GroupVoice.SourceAddress", "3191868")
	for _, b := range h.Buffers {
		if strings.HasPrefix(b.Name, "burst") && (b.Map[0] != 27 || b.Map[1] != 26) {
			t.Errorf("burst buffer map %v, want the octet pairs swapped", b.Map[:2])
		}
	}
	wantValue(t, r.Packets[5], "Hytera IPSC.Burst.DMR burst.Voice position", "E")
	wantValue(t, r.Packets[9], "Hytera IPSC.Code", "0x0A")
	if r.Packets[11].Tree[0].Value != "keepalive" {
		t.Errorf("packet 11 is %q, want keepalive", r.Packets[11].Tree[0].Value)
	}
	wantValue(t, r.Packets[14], "Hytera IPSC.Body.RedirectPort", "50001")
}

func TestMMDVM(t *testing.T) {
	t.Parallel()
	r := inspect(t, repo(t, "transport/mmdvm/testdata/mmdvmhost.txt"), Options{})
	protocols(t, r, "MMDVM modem serial", 16)
	noErrors(t, r)
	wantValue(t, r.Packets[2], "MMDVM frame.Command", "SetConfig")
	wantValue(t, r.Packets[2], "MMDVM frame.Payload.DMR", "true")
	cc := wantValue(t, r.Packets[2], "MMDVM frame.Payload.DMRColorCode", "7")
	if len(cc.Ranges) != 1 || cc.Ranges[0].Start%8 != 0 {
		t.Errorf("DMRColorCode ranges %+v", cc.Ranges)
	}
	d := r.Packets[8]
	wantValue(t, d, "MMDVM frame.Slot", "TS2")
	wantOctets(t, node(t, d, "MMDVM frame.Burst"), 4, 37)
	wantValue(t, d, "MMDVM frame.Burst.DMR burst.FullLinkControl.GroupVoice.GroupAddress", "91")

	p := inspect(t, repo(t, "transport/mmdvm/testdata/pistar.txt"), Options{})
	protocols(t, p, "MMDVM modem serial", 10)
	noErrors(t, p)

	stream := mustHex(t, strings.Fields(string(repo(t, "transport/mmdvm/testdata/mmdvmhost.txt")))[1]+strings.Fields(string(repo(t, "transport/mmdvm/testdata/mmdvmhost.txt")))[3])
	s := inspect(t, stream, Options{Format: "binary"})
	protocols(t, s, "MMDVM modem serial", 2)
}

func TestBursts(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		file, path, want string
	}{
		{"layer2/testdata/d-sms.bin", "DMR burst.Data packet (reassembled).Text message", `"TEST KI5VMF"`},
		{"layer2/testdata/m-sms.bin", "DMR burst.Data packet (reassembled).Text message", `"TEST KI5VMF"`},
		{"layer2/testdata/h-sms.bin", "DMR burst.Data packet (reassembled).Text message", `"TEST KI5VMF"`},
		{"sms/testdata/anytone_hsms.txt", "DMR burst.Data packet (reassembled).Text message", `"HYT TEST"`},
		{"sms/testdata/anytone_dmr_text.txt", "DMR burst.Data packet (reassembled).Text message", `"DMR TEST"`},
		{"data/testdata/anytone_sms_xpr.txt", "DMR burst.Data packet (reassembled).Text message", `"THE QUICK BROWN FOX`},
		{"layer2/testdata/anytone_aprs.txt", "DMR burst.Data packet (reassembled).Position", "34.21"},
		{"layer2/testdata/anytone_aprs.txt", "DMR burst.Data packet (reassembled).UDP datagram.Compressed", "true"},
		{"trunk/testdata/capplus_lcp_beacon.txt", "DMR burst.CSBK.Capacity Plus site status.RestLSN", ""},
		{"layer2/testdata/voice.bin", "DMR burst.Vocoder slot 1", ""},
	} {
		t.Run(filepath.Base(c.file)+"/"+c.want, func(t *testing.T) {
			t.Parallel()
			r := inspect(t, repo(t, c.file), Options{})
			found := false
			for _, p := range r.Packets {
				if p.Protocol != "DMR burst" {
					t.Fatalf("packet %d: protocol %q", p.Index, p.Protocol)
				}
				if n := findNode(p.Tree, c.path); n != nil && strings.HasPrefix(n.Value, c.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("no packet has %s = %s", c.path, c.want)
			}
		})
	}
	r := inspect(t, repo(t, "data/testdata/anytone_sms_xpr.txt"), Options{})
	resp := false
	for _, p := range r.Packets {
		if n := findNode(p.Tree, "DMR burst.Data packet (reassembled).Response"); n != nil {
			resp = true
		}
		if strings.Contains(strings.Join(p.Errors, " "), "UDP") {
			t.Errorf("packet %d: response packet decoded as UDP: %v", p.Index, p.Errors)
		}
	}
	if !resp {
		t.Error("no response packet found")
	}
}

func TestHDAP(t *testing.T) {
	t.Parallel()
	var tm hdap.TMPMessage
	tm.RequestID = 7
	tm.Destination = [4]byte{10, 0x30, 0xB4, 0x3C}
	tm.Source = [4]byte{10, 0, 0, 1}
	tm.SetTextUTF8(nil, []byte("hello dmrgo"))
	payload, err := tm.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := hdap.Message{Header: hdap.Header{Service: hdap.ServiceTMP}, Opcode: hdap.Opcode(hdap.TMPPrivateMessage), Payload: payload, End: 0x03}
	m.Seal()
	pkt, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	r := inspect(t, []byte(hex.EncodeToString(pkt)), Options{})
	protocols(t, r, "Hytera HDAP", 1)
	noErrors(t, r)
	p := r.Packets[0]
	wantOctets(t, node(t, p, "Hytera HDAP.Opcode"), 1, 3)
	node(t, p, "Hytera HDAP.TMP payload")
	if !strings.Contains(p.Summary, `"hello dmrgo"`) {
		t.Errorf("summary %q", p.Summary)
	}
	wantOctets(t, node(t, p, "Hytera HDAP.TMP payload.RequestID"), 5, 9)

	pkt[len(pkt)-2] ^= 0xFF
	bad := inspect(t, []byte(hex.EncodeToString(pkt)), Options{Protocol: "hdap"})
	if c := node(t, bad.Packets[0], "Hytera HDAP.Checksum"); c.Err == "" {
		t.Error("bad checksum not reported")
	}

	pkt[len(pkt)-2] ^= 0xFF
	pr := inspect(t, buildPcap(t, [][]byte{pkt}, 3005), Options{})
	protocols(t, pr, "Hytera HDAP", 1)
	noErrors(t, pr)
}

func udpFrame(payload []byte, port int) []byte {
	ip := make([]byte, 20+8+len(payload))
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:], uint16(len(ip)))
	ip[8] = 64
	ip[9] = 17
	copy(ip[12:], []byte{127, 0, 0, 1})
	copy(ip[16:], []byte{127, 0, 0, 2})
	binary.BigEndian.PutUint16(ip[20:], 40000)
	binary.BigEndian.PutUint16(ip[22:], uint16(port))
	binary.BigEndian.PutUint16(ip[24:], uint16(8+len(payload)))
	copy(ip[28:], payload)
	eth := make([]byte, 14, 14+len(ip))
	binary.BigEndian.PutUint16(eth[12:], 0x0800)
	return append(eth, ip...)
}

func buildPcap(t *testing.T, payloads [][]byte, port int) []byte {
	t.Helper()
	out := make([]byte, 24)
	binary.LittleEndian.PutUint32(out, 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(out[4:], 2)
	binary.LittleEndian.PutUint16(out[6:], 4)
	binary.LittleEndian.PutUint32(out[16:], 65535)
	binary.LittleEndian.PutUint32(out[20:], 1)
	for i, p := range payloads {
		f := udpFrame(p, port)
		h := make([]byte, 16)
		binary.LittleEndian.PutUint32(h, uint32(1700000000+i))
		binary.LittleEndian.PutUint32(h[8:], uint32(len(f)))
		binary.LittleEndian.PutUint32(h[12:], uint32(len(f)))
		out = append(append(out, h...), f...)
	}
	return out
}

func buildPcapng(payloads [][]byte, port int) []byte {
	block := func(typ uint32, body []byte) []byte {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		b := make([]byte, 8, 12+len(body))
		binary.LittleEndian.PutUint32(b, typ)
		binary.LittleEndian.PutUint32(b[4:], uint32(12+len(body)))
		b = append(b, body...)
		return binary.LittleEndian.AppendUint32(b, uint32(12+len(body)))
	}
	shb := make([]byte, 16)
	binary.LittleEndian.PutUint32(shb, 0x1a2b3c4d)
	binary.LittleEndian.PutUint16(shb[4:], 1)
	binary.LittleEndian.PutUint64(shb[8:], ^uint64(0))
	out := block(0x0a0d0d0a, shb)
	idb := make([]byte, 8)
	binary.LittleEndian.PutUint16(idb, 1)
	binary.LittleEndian.PutUint32(idb[4:], 65535)
	out = append(out, block(1, idb)...)
	for _, p := range payloads {
		f := udpFrame(p, port)
		epb := make([]byte, 20, 20+len(f))
		binary.LittleEndian.PutUint32(epb[12:], uint32(len(f)))
		binary.LittleEndian.PutUint32(epb[16:], uint32(len(f)))
		out = append(out, block(6, append(epb, f...))...)
	}
	return out
}

func fixturePackets(t *testing.T, path string) [][]byte {
	t.Helper()
	ds, errs := readHex(string(repo(t, path)))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	out := make([][]byte, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.data)
	}
	return out
}

func TestCaptures(t *testing.T) {
	t.Parallel()
	gw := fixturePackets(t, "transport/dmrd/testdata/dmrgateway-master.txt")
	hy := fixturePackets(t, "transport/hytera/testdata/built.txt")
	ip := fixturePackets(t, "transport/ipsc/testdata/xpr8400.txt")[121:161]
	for _, c := range []struct {
		name  string
		in    []byte
		n     int
		proto string
	}{
		{"pcap dmrd", buildPcap(t, gw, 62031), 29, ""},
		{"pcapng dmrd", buildPcapng(gw, 62031), 29, ""},
		{"pcap hytera", buildPcap(t, hy, 50001), 16, "Hytera IPSC"},
		{"pcapng ipsc", buildPcapng(ip, 50000), 40, "Motorola IPSC"},
	} {
		r := inspect(t, c.in, Options{})
		if len(r.Packets) != c.n || len(r.Errors) > 0 {
			t.Errorf("%s: %d packets, errors %v, want %d", c.name, len(r.Packets), r.Errors, c.n)
			continue
		}
		if !strings.HasPrefix(r.Input, "pcap") {
			t.Errorf("%s: input %q", c.name, r.Input)
		}
		noErrors(t, r)
		if c.proto != "" {
			protocols(t, r, c.proto, c.n)
		}
		if !strings.Contains(r.Packets[0].Meta, "127.0.0.1:40000 > 127.0.0.2") {
			t.Errorf("%s: meta %q", c.name, r.Packets[0].Meta)
		}
	}
	r := inspect(t, buildPcap(t, gw, 62031), Options{})
	wantValue(t, r.Packets[8], "DMRD.Burst.DMR burst.FullLinkControl.GroupVoice.GroupAddress", "91")
}

func TestDecodeErrors(t *testing.T) {
	t.Parallel()
	gw := fixturePackets(t, "transport/dmrd/testdata/dmrgateway-master.txt")
	short := gw[8][:52]
	r := inspect(t, []byte(hex.EncodeToString(short)), Options{})
	if p := r.Packets[0]; !strings.Contains(strings.Join(p.Errors, " "), "too short") || p.Tree[0].Err == "" {
		t.Errorf("short DMRD: errors %v", p.Errors)
	}

	bad := append([]byte(nil), gw[8]...)
	for i := 20; i < 30; i++ {
		bad[i] ^= 0xFF
	}
	r = inspect(t, []byte(hex.EncodeToString(bad)), Options{})
	if p := r.Packets[0]; !strings.Contains(strings.Join(p.Errors, " "), "FEC") {
		t.Errorf("corrupted burst: errors %v", p.Errors)
	}

	r = inspect(t, []byte("8000\nzz 0102\n"), Options{Format: "hex"})
	if len(r.Packets) != 2 || r.Packets[0].Errors == nil {
		t.Errorf("short IPSC: %+v", r.Packets)
	}
	r = inspect(t, []byte{0x01, 0x02, 0x03}, Options{Format: "binary", Protocol: "burst"})
	if !strings.Contains(r.Packets[0].Errors[0], "33 octets") {
		t.Errorf("short burst: %v", r.Packets[0].Errors)
	}
	r = inspect(t, []byte{0xd4, 0xc3, 0xb2, 0xa1, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0, 0, 1, 0, 0, 0, 1, 2, 3}, Options{})
	if len(r.Errors) == 0 {
		t.Error("truncated pcap: no error")
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	s := InspectJSON(repo(t, "transport/hytera/testdata/built.txt"), Options{})
	if !strings.HasPrefix(s, `{"input":"hex text","packets":[`) {
		t.Errorf("JSON starts %q", s[:min(60, len(s))])
	}
}

func TestDerived(t *testing.T) {
	t.Parallel()
	r := inspect(t, repo(t, "transport/ipsc/testdata/xpr8400.txt"), Options{})
	hdr := r.Packets[18]
	for path, want := range map[string]bool{
		"IPSC.DMR burst.Slot type.FEC":                 true,
		"IPSC.DMR burst.Slot type.ColorCode":           false,
		"IPSC.DMR burst.FullLinkControl.BurstDataType": true,
		"IPSC.DMR burst.FullLinkControl.FEC":           true,
		"IPSC.DMR burst.FullLinkControl.ProtectFlag":   false,
		"IPSC.Source": false,
	} {
		if n := node(t, hdr, path); n.Derived != want {
			t.Errorf("%s: derived %v, want %v", path, n.Derived, want)
		}
	}
	type plain struct {
		Sent uint8
		Size uint8 `dmr:"-"`
	}
	ns := walkPlain(reflect.ValueOf(plain{Sent: 1, Size: 2}))
	if len(ns) != 2 || ns[0].Derived || !ns[1].Derived {
		t.Errorf("walkPlain: %d nodes, want 2 with derived false, true", len(ns))
	}
	type stream struct {
		Sent uint8 `dmr:"u8"`
		Size uint8 `dmr:"-"`
	}
	ns = walkStream(reflect.ValueOf(stream{Sent: 1, Size: 1}), []byte{1}, 0, 0)
	if len(ns) != 2 || ns[0].Derived || !ns[1].Derived {
		t.Errorf("walkStream: %d nodes, want 2 with derived false, true", len(ns))
	}
}
