package pdu_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

type capPlusVector struct {
	recording, kind string
	octets          []byte
}

func capPlusVectors(tb testing.TB) []capPlusVector {
	tb.Helper()
	f, err := os.Open("testdata/capacity_plus.txt")
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	var out []capPlusVector
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Fields(line)
		if len(cols) != 3 {
			tb.Fatalf("bad line %q", line)
		}
		b, err := hex.DecodeString(cols[2])
		if err != nil {
			tb.Fatalf("bad line %q: %v", line, err)
		}
		out = append(out, capPlusVector{cols[0], cols[1], b})
	}
	if err := sc.Err(); err != nil {
		tb.Fatal(err)
	}
	return out
}

type capPlusWant struct {
	status    *pdu.CapacityPlusSiteStatusPDU
	activity  *pdu.CapacityPlusActivity
	neighbors *pdu.CapacityPlusNeighborsPDU
	preamble  *pdu.CapacityPlusPreamblePDU
	revert    *pdu.CapacityPlusDataRevertAnnouncementPDU
	rest      *pdu.CapacityPlusRestChannel
}

func capPlusStatus(slot2 bool, rest uint8, activity ...byte) *pdu.CapacityPlusSiteStatusPDU {
	p := &pdu.CapacityPlusSiteStatusPDU{Segment: pdu.CapacityPlusSegmentSingle, Slot2: slot2, RestLSN: rest}
	copy(p.Activity[:], activity)
	return p
}

func capPlusWants() map[string]capPlusWant {
	idle := func(rest uint8) *pdu.CapacityPlusActivity { return &pdu.CapacityPlusActivity{RestLSN: rest} }
	tg30 := func(rest, lsn uint8) *pdu.CapacityPlusActivity {
		a := &pdu.CapacityPlusActivity{RestLSN: rest, Voice: 1 << (lsn - 1)}
		a.Talkgroups[lsn-1] = 30
		return a
	}
	lcpNeighbors := func(slot2 bool) *pdu.CapacityPlusNeighborsPDU {
		return &pdu.CapacityPlusNeighborsPDU{Segment: pdu.CapacityPlusSegmentSingle, Slot2: slot2, RestLSN: 6, Site: 3, Count: 3, Neighbors: [6]byte{0x12, 0x23, 0x43}}
	}
	restNeighbors := func(slot2 bool) *pdu.CapacityPlusNeighborsPDU {
		return &pdu.CapacityPlusNeighborsPDU{Segment: pdu.CapacityPlusSegmentSingle, Slot2: slot2, RestLSN: 2, Site: 4, Count: 1, Neighbors: [6]byte{0xA1}}
	}
	pre := func(btf, rest uint8, src uint16) *pdu.CapacityPlusPreamblePDU {
		return &pdu.CapacityPlusPreamblePDU{Data: true, BlocksToFollow: btf, TargetAddress: 0xFFFF, RestLSN: rest, SourceAddress: src}
	}
	slc := func(unknown, rest, site uint8) *pdu.CapacityPlusRestChannel {
		return &pdu.CapacityPlusRestChannel{Marker: 0x10, Flags: unknown, RestLSN: rest, Site: site}
	}
	return map[string]capPlusWant{
		"be10c600000000000000f265": {status: capPlusStatus(false, 6), activity: idle(6)},
		"be10e600000000000000890d": {status: capPlusStatus(true, 6), activity: idle(6)},
		"bb10c61b122343000000cea3": {neighbors: lcpNeighbors(false)},
		"bb10e61b122343000000b5cb": {neighbors: lcpNeighbors(true)},
		"f100618460":               {rest: slc(0, 6, 3)},
		"be10c300000000000000badb": {status: capPlusStatus(false, 3), activity: idle(3)},
		"be10e300000000000000c1b3": {status: capPlusStatus(true, 3), activity: idle(3)},
		"be10c4000000000000007dc3": {status: capPlusStatus(false, 4), activity: idle(4)},
		"be10e40000000000000006ab": {status: capPlusStatus(true, 4), activity: idle(4)},
		"bd10800200ffff0407ed2d8a": {preamble: pre(2, 4, 2029)},
		"bd10800100ffff0407edf508": {preamble: pre(1, 4, 2029)},
		"bd10800000ffff0407ed4d69": {preamble: pre(0, 4, 2029)},
		"bd10800200ffff03082c7149": {preamble: pre(2, 3, 2092)},
		"f1003004f0":               {rest: slc(0, 3, 0)},
		"f100400240":               {rest: slc(0, 4, 0)},
		"f1024008a0":               {rest: slc(1, 4, 0)},
		"be10c200000000000000fd08": {status: capPlusStatus(false, 2), activity: idle(2)},
		"be10e2000000000000008660": {status: capPlusStatus(true, 2), activity: idle(2)},
		"bb10c221a100000000004d67": {neighbors: restNeighbors(false)},
		"bb10e221a10000000000360f": {neighbors: restNeighbors(true)},
		"f102220140":               {rest: slc(1, 2, 4)},
		"be10c2801e0000000000ccd6": {status: capPlusStatus(false, 2, 0x80, 0x1E), activity: tg30(2, 1)},
		"be10e2801e0000000000b7be": {status: capPlusStatus(true, 2, 0x80, 0x1E), activity: tg30(2, 1)},
		"be10e1401e000000000009de": {status: capPlusStatus(true, 1, 0x40, 0x1E), activity: tg30(1, 2)},
		"be10c1401e000000000072b6": {status: capPlusStatus(false, 1, 0x40, 0x1E), activity: tg30(1, 2)},
		"f1002102a0":               {rest: slc(0, 2, 2)},
		"f100110150":               {rest: slc(0, 1, 2)},
		"a91000002a3c02fc0000be07": {revert: &pdu.CapacityPlusDataRevertAnnouncementPDU{TargetAddress: 42, Window: 0x3C, Superframe: 2, Flags: 0xFC}},
		"a9100000004f02fc0000fe59": {revert: &pdu.CapacityPlusDataRevertAnnouncementPDU{Window: 0x4F, Superframe: 2, Flags: 0xFC}},
	}
}

func TestCapacityPlusRecordings(t *testing.T) {
	t.Parallel()
	wants := capPlusWants()
	seen := map[string]bool{}
	for _, v := range capPlusVectors(t) {
		key := hex.EncodeToString(v.octets)
		want, ok := wants[key]
		if !ok {
			t.Errorf("%s %s: no expectation", v.recording, key)
			continue
		}
		seen[key] = true
		switch v.kind {
		case "csbk":
			checkCapPlusCSBK(t, key, [12]byte(v.octets), want)
		case "slc":
			s := pdu.DecodeShortLC([5]byte(v.octets))
			if s.FEC.Uncorrectable {
				t.Errorf("%s: Short LC CRC fails", key)
			}
			p, ok := s.AsCapacityPlusRestChannel()
			if !ok || want.rest == nil || p != *want.rest {
				t.Errorf("%s: rest channel %+v %t, want %+v", key, p, ok, want.rest)
			}
		default:
			t.Fatalf("unknown kind %q", v.kind)
		}
	}
	for key := range wants {
		if !seen[key] {
			t.Errorf("%s: not in testdata", key)
		}
	}
}

func checkCapPlusCSBK(t *testing.T, key string, raw [12]byte, want capPlusWant) {
	t.Helper()
	c := pdu.DecodeCSBK(raw)
	if c.FEC.Uncorrectable || c.FID != enums.FIDMotorola || !c.LastBlock {
		t.Errorf("%s: decoded %+v", key, c)
		return
	}
	st, okSt := c.AsCapacityPlusSiteStatus()
	nb, okNb := c.AsCapacityPlusNeighbors()
	pr, okPr := c.AsCapacityPlusPreamble()
	rv, okRv := c.AsCapacityPlusDataRevertAnnouncement()
	_, okGr := c.AsCapacityPlusDataRevertGrant()
	if okSt != (want.status != nil) || okNb != (want.neighbors != nil) || okPr != (want.preamble != nil) || okRv != (want.revert != nil) || okGr {
		t.Errorf("%s: As results %t %t %t %t %t", key, okSt, okNb, okPr, okRv, okGr)
		return
	}
	switch {
	case okSt:
		if st != *want.status {
			t.Errorf("%s: status %+v, want %+v", key, st, *want.status)
		}
		var a pdu.CapacityPlusSiteStatusAssembler
		if done, err := a.Add(&st); !done || err != nil {
			t.Fatalf("%s: Add %t %v", key, done, err)
		}
		act, err := a.Activity()
		if err != nil || act != *want.activity {
			t.Errorf("%s: activity %+v %v, want %+v", key, act, err, *want.activity)
		}
	case okNb:
		if nb != *want.neighbors {
			t.Errorf("%s: neighbors %+v, want %+v", key, nb, *want.neighbors)
		}
	case okPr:
		if pr != *want.preamble {
			t.Errorf("%s: preamble %+v, want %+v", key, pr, *want.preamble)
		}
	case okRv:
		if rv != *want.revert {
			t.Errorf("%s: revert %+v, want %+v", key, rv, *want.revert)
		}
	}
	re := c
	if got := must(re.Encode()); got != raw {
		t.Errorf("%s: re-encoded %x", key, got)
	}
}

func TestCapacityPlusNeighbor(t *testing.T) {
	t.Parallel()
	p := pdu.CapacityPlusNeighborsPDU{Count: 3, Neighbors: [6]byte{0x12, 0x23, 0x43, 0x55}}
	want := [][2]uint8{{1, 2}, {2, 3}, {4, 3}}
	for i, w := range want {
		site, rest, ok := p.Neighbor(i)
		if !ok || site != w[0] || rest != w[1] {
			t.Errorf("Neighbor(%d) = %d %d %t, want %d %d", i, site, rest, ok, w[0], w[1])
		}
	}
	for _, i := range []int{-1, 3, 6} {
		if _, _, ok := p.Neighbor(i); ok {
			t.Errorf("Neighbor(%d) ok", i)
		}
	}
	p.Count = 7
	if _, _, ok := p.Neighbor(6); ok {
		t.Error("Neighbor(6) ok with Count 7")
	}
}

func TestCapacityPlusChannel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lsn      uint8
		repeater uint8
		slot     enums.Slot
		ok       bool
	}{
		{0, 0, enums.SlotUnknown, false},
		{1, 1, enums.Slot1, true},
		{2, 1, enums.Slot2, true},
		{3, 2, enums.Slot1, true},
		{10, 5, enums.Slot2, true},
		{16, 8, enums.Slot2, true},
		{17, 0, enums.SlotUnknown, false},
	}
	for _, tt := range tests {
		r, s, ok := pdu.CapacityPlusChannel(tt.lsn)
		if r != tt.repeater || s != tt.slot || ok != tt.ok {
			t.Errorf("CapacityPlusChannel(%d) = %d %v %t", tt.lsn, r, s, ok)
		}
	}
}

func capPlusBlocks(t *testing.T, rest uint8, stream []byte) []pdu.CapacityPlusSiteStatusPDU {
	t.Helper()
	n := (len(stream) + 6) / 7
	if n == 0 {
		n = 1
	}
	out := make([]pdu.CapacityPlusSiteStatusPDU, n)
	for i := range out {
		seg := pdu.CapacityPlusSegmentContinuation
		switch {
		case n == 1:
			seg = pdu.CapacityPlusSegmentSingle
		case i == 0:
			seg = pdu.CapacityPlusSegmentFirst
		case i == n-1:
			seg = pdu.CapacityPlusSegmentLast
		}
		out[i] = pdu.CapacityPlusSiteStatusPDU{Segment: seg, RestLSN: rest}
		copy(out[i].Activity[:], stream[min(7*i, len(stream)):])
	}
	return out
}

func TestCapacityPlusSiteStatusAssembler(t *testing.T) {
	t.Parallel()
	full := make([]byte, 0, 53)
	full = append(full, 0xFF)
	for i := range 8 {
		full = append(full, byte(10+i))
	}
	full = append(full, 0xFF)
	for i := range 8 {
		full = append(full, byte(20+i))
	}
	full = append(full, 0x90, 0xFF)
	for i := range 8 {
		full = append(full, 0x10, byte(i))
	}
	full = append(full, 0xFF)
	for i := range 8 {
		full = append(full, 0x20, byte(i))
	}
	var fullAct pdu.CapacityPlusActivity
	fullAct.RestLSN, fullAct.Voice, fullAct.Radio, fullAct.Options = 5, 0xFFFF, 0xFFFF, 0x90
	for i := range 8 {
		fullAct.Talkgroups[i], fullAct.Talkgroups[8+i] = byte(10+i), byte(20+i)
		fullAct.Radios[i], fullAct.Radios[8+i] = 0x1000|uint16(i), 0x2000|uint16(i)
	}

	mixed := pdu.CapacityPlusActivity{RestLSN: 5, Voice: 1<<2 | 1<<8, Options: 0x80, Radio: 1<<0 | 1<<5}
	mixed.Talkgroups[2], mixed.Talkgroups[8] = 23, 106
	mixed.Radios[0], mixed.Radios[5] = 112, 21001

	tests := []struct {
		name   string
		stream []byte
		want   pdu.CapacityPlusActivity
		err    error
	}{
		{"idle", nil, pdu.CapacityPlusActivity{RestLSN: 5}, nil},
		{"mixed", []byte{0x20, 23, 0x80, 106, 0x80, 0x84, 0, 112, 0x52, 0x09, 0x00}, mixed, nil},
		{"full", full, fullAct, nil},
	}
	for _, tt := range tests {
		var a pdu.CapacityPlusSiteStatusAssembler
		blocks := capPlusBlocks(t, 5, tt.stream)
		for i := range blocks {
			done, err := a.Add(&blocks[i])
			if err != nil || done != (i == len(blocks)-1) {
				t.Fatalf("%s: block %d: %t %v", tt.name, i, done, err)
			}
		}
		got, err := a.Activity()
		if !errors.Is(err, tt.err) || got != tt.want {
			t.Errorf("%s: %+v %v, want %+v", tt.name, got, err, tt.want)
		}
		if len(blocks) != (len(tt.stream)+6)/7 && len(tt.stream) > 0 {
			t.Errorf("%s: %d blocks", tt.name, len(blocks))
		}
	}
	if n := len(capPlusBlocks(t, 5, full)); n != pdu.CapacityPlusMaxBlocks {
		t.Errorf("full status takes %d blocks, want %d", n, pdu.CapacityPlusMaxBlocks)
	}
}

func TestCapacityPlusSiteStatusAssemblerErrors(t *testing.T) {
	t.Parallel()
	var a pdu.CapacityPlusSiteStatusAssembler
	if _, err := a.Activity(); !errors.Is(err, pdu.ErrCapacityPlusIncomplete) {
		t.Errorf("empty Activity: %v", err)
	}
	cont := pdu.CapacityPlusSiteStatusPDU{Segment: pdu.CapacityPlusSegmentContinuation}
	last := pdu.CapacityPlusSiteStatusPDU{Segment: pdu.CapacityPlusSegmentLast}
	first := pdu.CapacityPlusSiteStatusPDU{Segment: pdu.CapacityPlusSegmentFirst, RestLSN: 3}
	for _, p := range []*pdu.CapacityPlusSiteStatusPDU{&cont, &last} {
		if done, err := a.Add(p); done || !errors.Is(err, pdu.ErrCapacityPlusSequence) {
			t.Errorf("%v without first: %t %v", p.Segment, done, err)
		}
	}
	if done, err := a.Add(&first); done || err != nil {
		t.Fatalf("first: %t %v", done, err)
	}
	if _, err := a.Activity(); !errors.Is(err, pdu.ErrCapacityPlusIncomplete) {
		t.Errorf("Activity of an open status: %v", err)
	}
	for i := 1; i < pdu.CapacityPlusMaxBlocks; i++ {
		if _, err := a.Add(&cont); err != nil {
			t.Fatalf("continuation %d: %v", i, err)
		}
	}
	if _, err := a.Add(&last); !errors.Is(err, pdu.ErrCapacityPlusSequence) {
		t.Errorf("block %d: %v", pdu.CapacityPlusMaxBlocks+1, err)
	}

	short := pdu.CapacityPlusSiteStatusPDU{Segment: pdu.CapacityPlusSegmentSingle, RestLSN: 3, Activity: [7]byte{0xFF, 1, 2, 3, 4, 5, 6}}
	if _, err := a.Add(&short); err != nil {
		t.Fatal(err)
	}
	act, err := a.Activity()
	want := pdu.CapacityPlusActivity{RestLSN: 3, Voice: 0x3F, Talkgroups: [16]uint8{1, 2, 3, 4, 5, 6}}
	if !errors.Is(err, pdu.ErrCapacityPlusIncomplete) || act != want {
		t.Errorf("truncated: %+v %v, want %+v", act, err, want)
	}
	a.Reset()
	if _, err := a.Activity(); !errors.Is(err, pdu.ErrCapacityPlusIncomplete) {
		t.Errorf("Activity after Reset: %v", err)
	}
}

func TestCapacityPlusVoiceGrants(t *testing.T) {
	t.Parallel()
	so := layer3.ServiceOptions{IsEmergency: true}
	g := pdu.CapacityPlusGroupVoiceGrant{ServiceOptions: so, GroupAddress: 14, RestLSN: 17, SourceAddress: 1400}
	payload := must(pdu.EncodePayload(&g))
	lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader, FLCO: pdu.FLCOCapacityPlusGroupVoiceGrant, FID: enums.FIDMotorola, RawPayload: payload}
	raw := must(lc.Encode())
	back := pdu.DecodeFullLinkControl(raw, enums.DataTypeVoiceLCHeader)
	if back.FEC.Uncorrectable {
		t.Fatal("group grant RS fails")
	}
	if got, ok := back.AsCapacityPlusGroupVoiceGrant(); !ok || got != g {
		t.Errorf("group grant %+v %t, want %+v", got, ok, g)
	}
	if _, ok := back.AsCapacityPlusPrivateVoiceGrant(); ok {
		t.Error("group grant read as private")
	}
	if want := [7]byte{0x80, 0, 0, 14, 17, 0x05, 0x78}; payload != want {
		t.Errorf("group payload %x, want %x", payload, want)
	}

	p := pdu.CapacityPlusPrivateVoiceGrant{TargetAddress: 21001, RestLSN: 4, SourceAddress: 21002}
	lc = pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader, FLCO: pdu.FLCOCapacityPlusPrivateVoiceGrant, FID: enums.FIDMotorola, RawPayload: must(pdu.EncodePayload(&p))}
	back = pdu.DecodeFullLinkControl(must(lc.Encode()), enums.DataTypeVoiceLCHeader)
	if got, ok := back.AsCapacityPlusPrivateVoiceGrant(); !ok || got != p {
		t.Errorf("private grant %+v %t, want %+v", got, ok, p)
	}
	std := pdu.FullLinkControl{FLCO: pdu.FLCOCapacityPlusGroupVoiceGrant}
	if _, ok := std.AsCapacityPlusGroupVoiceGrant(); ok {
		t.Error("standard FID read as a Capacity Plus grant")
	}
	if _, err := pdu.EncodePayload(&pdu.CapacityPlusGroupVoiceGrant{RestLSN: 32}); err == nil {
		t.Error("RestLSN 32 encodes")
	}
}

func TestCapacityPlusDataRevertGrant(t *testing.T) {
	t.Parallel()
	p := pdu.CapacityPlusDataRevertGrantPDU{TargetAddress: 42, Window: 7, Superframe: 2}
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKCapacityPlusDataRevertGrant, FID: enums.FIDMotorola, RawPayload: must(pdu.EncodePayload(&p))}
	back := pdu.DecodeCSBK(must(c.Encode()))
	if got, ok := back.AsCapacityPlusDataRevertGrant(); !ok || got != p {
		t.Errorf("grant %+v %t, want %+v", got, ok, p)
	}
	if _, ok := back.AsCapacityPlusSiteStatus(); ok {
		t.Error("grant read as site status")
	}
}

func TestCapacityPlusRestChannelMarker(t *testing.T) {
	t.Parallel()
	s := pdu.ShortLC{SLCO: pdu.SLCOCapacityPlusRestChannel, RawPayload: [3]byte{0x11, 0x06, 0x18}}
	if _, ok := s.AsCapacityPlusRestChannel(); ok {
		t.Error("marker 0x11 read as a rest channel")
	}
	s.SLCO = enums.SLCOActivityUpdate
	s.RawPayload[0] = 0x10
	if _, ok := s.AsCapacityPlusRestChannel(); ok {
		t.Error("SLCO 1 read as a rest channel")
	}
}

func TestCapacityPlusSegmentString(t *testing.T) {
	t.Parallel()
	for s, want := range map[pdu.CapacityPlusSegment]string{
		pdu.CapacityPlusSegmentContinuation: "Continuation",
		pdu.CapacityPlusSegmentLast:         "Last",
		pdu.CapacityPlusSegmentFirst:        "First",
		pdu.CapacityPlusSegmentSingle:       "Single",
		4:                                   "Unknown CapacityPlusSegment: 4",
	} {
		if got := s.String(); got != want {
			t.Errorf("%d: %q, want %q", s, got, want)
		}
	}
}

func TestCapacityPlusCSBKString(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		op         pdu.CSBKOpcode
		name, view string
	}{
		{pdu.CSBKCapacityPlusSiteStatus, "Capacity Plus Site Status", "CapacityPlusSiteStatusPDU{"},
		{pdu.CSBKCapacityPlusNeighbors, "Capacity Plus Neighbors", "CapacityPlusNeighborsPDU{"},
		{pdu.CSBKCapacityPlusDataRevertAnnouncement, "Capacity Plus Data Revert Announcement", "CapacityPlusDataRevertAnnouncementPDU{"},
		{pdu.CSBKCapacityPlusPreamble, "", "CapacityPlusPreamblePDU{"},
		{pdu.CSBKCapacityPlusDataRevertGrant, "", "CapacityPlusDataRevertGrantPDU{"},
	} {
		c := pdu.CSBK{Opcode: tc.op, FID: enums.FIDMotorola, RawPayload: [8]byte{0xC4, 1, 2, 3, 4, 5, 6, 7}}
		s := c.String()
		if strings.Contains(s, "Unknown CSBKOpcode") || !strings.Contains(s, tc.name) || !strings.Contains(s, tc.view) {
			t.Errorf("opcode %#x: %s", byte(tc.op), s)
		}
	}
}

func forumOctets(t *testing.T, s string) [12]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 12 {
		t.Fatalf("bad vector %q", s)
	}
	return [12]byte(b)
}

func TestCapacityPlusForumMultiBlock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		first, last string
		rest        uint8
		groups      map[uint8]uint8
		radios      map[uint8]uint16
	}{
		{"BE10A2000080B000010180A1", "BE1062D80001000000000373", 2, nil, map[uint8]uint16{1: 1, 3: 472, 4: 1}},
		{"BE10A200008090000100A7CE", "BE106201000000000000C380", 2, nil, map[uint8]uint16{1: 1, 4: 1}},
		{"BE10820000808703E702552A", "BE10428701FB028F0000DE3E", 2, nil, map[uint8]uint16{1: 999, 6: 647, 7: 507, 8: 655}},
		{"BE10820000808503E702B842", "BE104287028F000000007E5D", 2, nil, map[uint8]uint16{1: 999, 6: 647, 8: 655}},
		{"BE1084400100802000DE6212", "BE1044000000000000008042", 4, map[uint8]uint8{2: 1}, map[uint8]uint16{3: 222}},
	}
	for _, tt := range tests {
		var a pdu.CapacityPlusSiteStatusAssembler
		for i, s := range []string{tt.first, tt.last} {
			c := pdu.DecodeCSBK(forumOctets(t, s))
			p, ok := c.AsCapacityPlusSiteStatus()
			if c.FEC.Uncorrectable || !ok || p.RestLSN != tt.rest {
				t.Fatalf("%s: block %d: %+v %t", tt.first, i, p, ok)
			}
			wantSeg := pdu.CapacityPlusSegmentFirst
			if i == 1 {
				wantSeg = pdu.CapacityPlusSegmentLast
			}
			done, err := a.Add(&p)
			if p.Segment != wantSeg || err != nil || done != (i == 1) {
				t.Fatalf("%s: block %d: %v %t %v", tt.first, i, p.Segment, done, err)
			}
		}
		act, err := a.Activity()
		if err != nil || act.RestLSN != tt.rest {
			t.Fatalf("%s: %+v %v", tt.first, act, err)
		}
		for lsn := uint8(1); lsn <= 16; lsn++ {
			tg, okG := act.Talkgroup(lsn)
			if want, ok := tt.groups[lsn]; okG != ok || tg != want {
				t.Errorf("%s: LSN %d talkgroup %d %t, want %d %t", tt.first, lsn, tg, okG, want, ok)
			}
			r, okR := act.RadioTarget(lsn)
			if want, ok := tt.radios[lsn]; okR != ok || r != want {
				t.Errorf("%s: LSN %d radio %d %t, want %d %t", tt.first, lsn, r, okR, want, ok)
			}
		}
	}
}

func TestCapacityPlusForumPrivateGrant(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		lc   string
		rest uint8
	}{{"07100000520904520A50ED7F", 4}, {"07100000520903520AEDFD71", 3}} {
		lc := pdu.DecodeFullLinkControl(forumOctets(t, tt.lc), enums.DataTypeVoiceLCHeader)
		g, ok := lc.AsCapacityPlusPrivateVoiceGrant()
		want := pdu.CapacityPlusPrivateVoiceGrant{TargetAddress: 21001, RestLSN: tt.rest, SourceAddress: 21002}
		if lc.FEC.Uncorrectable || !ok || g != want {
			t.Errorf("%s: %+v %t, want %+v", tt.lc, g, ok, want)
		}
		if _, ok := lc.AsCapacityPlusGroupVoiceGrant(); ok {
			t.Errorf("%s: read as a group grant", tt.lc)
		}
	}
}
