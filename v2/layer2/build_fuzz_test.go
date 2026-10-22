package layer2_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func withoutFEC(d any) any {
	v := reflect.ValueOf(d).Elem()
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	if f := c.FieldByName("FEC"); f.IsValid() {
		f.SetZero()
	}
	return c.Interface()
}

func dataEncodeErr(d layer2.Data) error {
	var err error
	switch e := d.(type) {
	case interface{ Encode() ([12]byte, error) }:
		_, err = e.Encode()
	case interface{ Encode() ([18]byte, error) }:
		_, err = e.Encode()
	case interface{ Encode() ([24]byte, error) }:
		_, err = e.Encode()
	default:
		err = errors.New("no Encode method")
	}
	return err
}

type dataSeed struct {
	dt, cc, src byte
	payload     []byte
}

func dataSeeds(tb testing.TB) []dataSeed {
	tb.Helper()
	var seeds []dataSeed
	var b layer2.Burst
	for _, r := range captureFrames(tb) {
		if b.DecodeFromBytes(r) != nil || b.Kind != layer2.KindData {
			continue
		}
		dt := b.SlotType.DataType
		var p []byte
		switch dt {
		case enums.DataTypeRate34:
			v, _ := r.Rate34Payload()
			p = v[:]
		case enums.DataTypeRate1:
			v, _ := r.Rate1Payload()
			p = v[:]
		default:
			v, _ := r.BPTCPayload()
			p = v[:]
		}
		seeds = append(seeds, dataSeed{byte(dt), byte(b.SlotType.ColorCode), 0, p})
	}
	return append(seeds, dataSeed{byte(enums.DataTypeReserved), 16, 4, []byte{1, 2, 3}})
}

func buildPayload(t *testing.T, p *[24]byte, dt enums.DataType, cc enums.ColorCode, src layer2.SyncSource) (layer2.RawBurst, bool) {
	t.Helper()
	_, sync, srcOK := src.SyncPatterns()
	wantErr := !srcOK || cc > 15
	var r layer2.RawBurst
	var err error
	switch dt {
	case enums.DataTypeRate34:
		r, err = layer2.BuildRate34DataBurst([18]byte(p[:18]), cc, src)
	case enums.DataTypeRate1:
		r, err = layer2.BuildRate1DataBurst(*p, cc, src)
	default:
		r, err = layer2.BuildDataBurst([12]byte(p[:12]), dt, cc, src)
		wantErr = wantErr || dt.IsReserved()
	}
	if wantErr != (err != nil) {
		t.Fatalf("build of data type %v, color code %d, source %d: %v", dt, cc, src, err)
	}
	if err != nil {
		var fre *dmrgo.FieldRangeError
		if srcOK && !dt.IsReserved() && !errors.As(err, &fre) {
			t.Fatalf("color code %d: %v, want a *dmrgo.FieldRangeError", cc, err)
		}
		return r, false
	}
	switch dt {
	case enums.DataTypeRate34:
		if got, res := r.Rate34Payload(); got != [18]byte(p[:18]) || !clean(res) {
			t.Fatalf("Rate34Payload() = %x, %+v, want %x", got, res, p[:18])
		}
	case enums.DataTypeRate1:
		if got, _ := r.Rate1Payload(); got != *p {
			t.Fatalf("Rate1Payload() = %x, want %x", got, *p)
		}
	default:
		if got, res := r.BPTCPayload(); got != [12]byte(p[:12]) || !clean(res) {
			t.Fatalf("BPTCPayload() = %x, %+v, want %x", got, res, p[:12])
		}
		if got, ok := r.MMDVMHostPayload(dt); got != [12]byte(p[:12]) || ok != layer2.MMDVMHostAccepts(dt, got) {
			t.Fatalf("MMDVMHostPayload(%v) = %x, %v", dt, got, ok)
		}
	}
	if got, d := r.Sync(); got != sync || d != 0 {
		t.Fatalf("Sync() = %v, %d, want %v", got, d, sync)
	}
	if st := r.SlotType(); st.DataType != dt || st.ColorCode != cc || !clean(st.FEC) {
		t.Fatalf("SlotType() = %+v, want data type %v, color code %d", st, dt, cc)
	}
	return r, true
}

var mmdvmChecked = []enums.DataType{
	enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeCSBK, enums.DataTypeMBCHeader,
	enums.DataTypeDataHeader,
}

func checkBuildBurst(t *testing.T, d layer2.Data, cc enums.ColorCode, src layer2.SyncSource) {
	t.Helper()
	_, sync, _ := src.SyncPatterns()
	dt := d.DataType()
	built, err := layer2.BuildBurst(d, cc, src)
	if encErr := dataEncodeErr(d); (encErr != nil) != (err != nil) {
		t.Fatalf("BuildBurst(%s): %v, but its Encode returns %v", d.String(), err, encErr)
	}
	if err != nil {
		return
	}
	var b layer2.Burst
	if err := b.DecodeFromBytes(built); err != nil {
		t.Fatalf("decode of BuildBurst(%s): %v", d.String(), err)
	}
	if agg := b.FEC.Aggregate(); !clean(agg) {
		t.Fatalf("BuildBurst(%s) decodes with FEC %+v", d.String(), b.FEC)
	}
	if b.Kind != layer2.KindData || b.SyncPattern != sync || b.SlotType.DataType != dt || b.SlotType.ColorCode != cc ||
		b.BPTCReserved != 0 {
		t.Fatalf("BuildBurst decodes as %v, %v, %+v, reserved %d", b.Kind, b.SyncPattern, b.SlotType, b.BPTCReserved)
	}
	if b.Data == nil || !reflect.DeepEqual(withoutFEC(d), withoutFEC(b.Data)) {
		t.Fatalf("BuildBurst(%s) decodes to %v", d.String(), b.Data)
	}
	if _, ok := built.MMDVMHostPayload(dt); slices.Contains(mmdvmChecked, dt) && !ok {
		t.Fatalf("MMDVMHost rejects BuildBurst(%s)", d.String())
	}
	again, err := b.Encode()
	if err != nil || again != built {
		t.Fatalf("decoded BuildBurst(%s) encodes to %x, %v, want %x", d.String(), again, err, built)
	}
}

func checkBuilders(t *testing.T, dtv, ccv, srcv byte, payload []byte) {
	var p [24]byte
	copy(p[:], payload)
	dt := enums.DataType(dtv & 0xF)
	cc := enums.ColorCode(ccv & 0x1F)
	src := layer2.SyncSource(srcv % 5)
	r, ok := buildPayload(t, &p, dt, cc, src)
	if !ok {
		return
	}
	_, sync, _ := src.SyncPatterns()
	var b layer2.Burst
	_ = b.DecodeFromBytes(r)
	if b.Kind != layer2.KindData || b.SyncPattern != sync || !clean(b.FEC.Payload) || !clean(b.FEC.SlotType) {
		t.Fatalf("built burst decodes as %v, %v, FEC %+v", b.Kind, b.SyncPattern, b.FEC)
	}
	if b.Data != nil {
		checkBuildBurst(t, b.Data, cc, src)
	}
}

func FuzzBuildBurst(f *testing.F) {
	for _, s := range dataSeeds(f) {
		f.Add(s.dt, s.cc, s.src, s.payload)
	}
	f.Fuzz(checkBuilders)
}

func checkMMDVMHostSeal(t *testing.T, dtv, _, _ byte, payload []byte) {
	var p [12]byte
	copy(p[:], payload)
	dt := enums.DataType(dtv & 0xF)
	sealed := p
	if err := layer2.MMDVMHostSeal(dt, &sealed); err != nil {
		if sealed != p {
			t.Fatalf("failed MMDVMHostSeal(%v) changed %x to %x", dt, p, sealed)
		}
		if layer2.MMDVMHostAccepts(dt, p) {
			t.Fatalf("MMDVMHostSeal(%v) fails, but MMDVMHostAccepts takes %x", dt, p)
		}
		return
	}
	if !layer2.MMDVMHostAccepts(dt, sealed) {
		t.Fatalf("MMDVMHostAccepts(%v) rejects sealed %x", dt, sealed)
	}
	twice := sealed
	if err := layer2.MMDVMHostSeal(dt, &twice); err != nil || twice != sealed {
		t.Fatalf("second MMDVMHostSeal(%v) of %x: %x, %v", dt, sealed, twice, err)
	}
	if layer2.MMDVMHostAccepts(dt, p) && dt != enums.DataTypePIHeader && sealed != p {
		t.Fatalf("MMDVMHostSeal(%v) changed accepted %x to %x", dt, p, sealed)
	}
}

func FuzzMMDVMHostSeal(f *testing.F) {
	for _, s := range dataSeeds(f) {
		f.Add(s.dt, s.cc, s.src, s.payload)
	}
	f.Fuzz(checkMMDVMHostSeal)
}

type voiceBurstSeed struct {
	frames   []byte
	pos, emb byte
	embedded []byte
	src      byte
}

func voiceBurstSeeds() []voiceBurstSeed {
	seeds := make([]voiceBurstSeed, 8)
	for pos := range seeds {
		seeds[pos] = voiceBurstSeed{[]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0x80}, byte(pos), 0x11, []byte{1, 2, 3, 4}, byte(pos % 5)}
	}
	return seeds
}

func checkBuildVoiceBurst(t *testing.T, framesv []byte, posv, embv byte, embedded []byte, srcv byte) {
	var fv [21]byte
	copy(fv[:], framesv)
	frames := ambeFrames(fv[:])
	pos := enums.VoiceBurstType(posv % 8)
	emb := pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(embv & 0x1F), PreemptionAndPowerControlIndicator: embv&0x20 != 0}
	var data [4]byte
	copy(data[:], embedded)
	src := layer2.SyncSource(srcv % 5)
	voice, _, srcOK := src.SyncPatterns()

	r, err := layer2.BuildVoiceBurst(frames, pos, emb, data, src)
	wantErr := !srcOK || pos < enums.VoiceBurstA || pos > enums.VoiceBurstF || pos != enums.VoiceBurstA && emb.ColorCode > 15
	if wantErr != (err != nil) {
		t.Fatalf("BuildVoiceBurst(pos %v, emb %+v, source %d): %v", pos, emb, src, err)
	}
	if err != nil {
		return
	}
	var b layer2.Burst
	if err := b.DecodeFromBytesWith(r, &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: pos}); err != nil {
		t.Fatalf("decode at %v: %v", pos, err)
	}
	if got, res := b.AMBEFrames(); got != frames || !clean(res) {
		t.Fatalf("AMBEFrames() = %x, %+v, want %x", got, res, frames)
	}
	if pos == enums.VoiceBurstA {
		if b.SyncPattern != voice || b.SyncDistance != 0 {
			t.Fatalf("burst A has SYNC %v at %d, want %v", b.SyncPattern, b.SyncDistance, voice)
		}
		var auto layer2.Burst
		if err := auto.DecodeFromBytes(r); err != nil || auto.Kind != layer2.KindVoice || auto.VoiceBurst != enums.VoiceBurstA {
			t.Fatalf("burst A decodes as %v %v: %v", auto.Kind, auto.VoiceBurst, err)
		}
	} else {
		lcss, _ := pos.LCSS()
		got := b.EmbeddedSignalling
		if !b.HasEmbeddedSignalling() || got.ColorCode != emb.ColorCode || got.LCSS != lcss ||
			got.PreemptionAndPowerControlIndicator != emb.PreemptionAndPowerControlIndicator || !clean(got.FEC) {
			t.Fatalf("EMB %+v at %v, want %+v with LCSS %v", got, pos, emb, lcss)
		}
		if b.EmbeddedSignallingData != data || r.EmbeddedData() != data {
			t.Fatalf("embedded data %x, want %x", b.EmbeddedSignallingData, data)
		}
	}
	want := r
	if b.HasReverseChannel() {
		want.SetEmbeddedData(must(layer2.EncodeEmbeddedRC(&b.ReverseChannel)))
		if clean(b.ReverseChannel.FEC) && want != r {
			t.Fatalf("RC %+v of embedded data %x encodes to %x", b.ReverseChannel, data, want.EmbeddedData())
		}
	}
	if again, err := b.Encode(); err != nil || again != want {
		t.Fatalf("decoded voice burst encodes to %x, %v, want %x", again, err, want)
	}
}

func FuzzBuildVoiceBurst(f *testing.F) {
	for _, s := range voiceBurstSeeds() {
		f.Add(s.frames, s.pos, s.emb, s.embedded, s.src)
	}
	f.Fuzz(checkBuildVoiceBurst)
}

func checkCallLC(t *testing.T, b *layer2.Burst, err error, lc pdu.FullLinkControl, dt enums.DataType) {
	t.Helper()
	lc.BurstDataType = dt
	flc, ok := b.Data.(*pdu.FullLinkControl)
	if err != nil || !ok || b.SlotType.DataType != dt || !reflect.DeepEqual(withoutFEC(flc), withoutFEC(&lc)) {
		t.Fatalf("%v burst decodes to %v, %v, want %s", dt, b.Data, err, lc.String())
	}
}

type transmitterSeed struct {
	lc      []byte
	cc, src byte
	frames  []byte
}

func transmitterSeeds() []transmitterSeed {
	lc := groupLC(enums.FIDStandardized)
	lc.BurstDataType = enums.DataTypeVoiceLCHeader
	cw := must(lc.Encode())
	return []transmitterSeed{
		{cw[:], 1, 0, make([]byte, 7*20)},
		{cw[:], 15, 3, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x80}},
		{cw[:], 2, 1, []byte{}},
	}
}

func checkVoiceTransmitter(t *testing.T, lcv []byte, ccv, srcv byte, framesv []byte) {
	var cw [12]byte
	copy(cw[:], lcv)
	lc := pdu.DecodeFullLinkControl(cw, enums.DataTypeVoiceLCHeader)
	opts := &layer2.VoiceTransmitterOptions{
		LC:        lc,
		ColorCode: enums.ColorCode(ccv & 0x1F),
		Source:    layer2.SyncSource(srcv % 5),
		Silence:   vocoder.AMBESilenceSlot(),
	}
	_, encErr := lc.Encode()
	tx, err := layer2.NewVoiceTransmitter(opts)
	voiceSync, _, srcOK := opts.Source.SyncPatterns()
	if wantErr := encErr != nil || opts.ColorCode > 15 || !srcOK; wantErr != (err != nil) {
		t.Fatalf("NewVoiceTransmitter(LC %s, color code %d, source %d): %v", lc.String(), opts.ColorCode, opts.Source, err)
	}
	if err != nil {
		return
	}
	n := min(len(framesv)/7, 40)
	frames := make([]vocoder.AMBEFrame, n)
	for i := range frames {
		copy(frames[i][:], framesv[7*i:])
		frames[i][6] &= 0x80
	}
	bursts := tx.Headers()
	headers := len(bursts)
	voice, err := tx.AddFrames(frames...)
	if err != nil {
		t.Fatalf("AddFrames: %v", err)
	}
	end, err := tx.Terminate()
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	bursts = append(append(bursts, voice...), end...)

	silence, _ := vocoder.DecodeAMBE72(vocoder.AMBESilenceSlot())
	var b layer2.Burst
	var rx layer2.VoiceReceiver
	var got []vocoder.AMBEFrame
	lcs := 0
	for i, r := range bursts {
		err := b.DecodeFromBytes(r)
		switch {
		case i == 0:
			checkCallLC(t, &b, err, lc, enums.DataTypeVoiceLCHeader)
			continue
		case i == len(bursts)-1:
			checkCallLC(t, &b, err, lc, enums.DataTypeTerminatorWithLC)
			continue
		case i < headers:
			continue
		}
		wantPos := enums.VoiceBurstA + enums.VoiceBurstType((i-headers)%6)
		if err := b.DecodeFromBytesWith(r, &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: wantPos}); err != nil {
			t.Fatalf("voice burst %d: %v", i, err)
		}
		if wantPos == enums.VoiceBurstA && (b.SyncPattern != voiceSync || b.SyncDistance != 0) {
			t.Fatalf("voice burst %d has SYNC %v at %d, want %v", i-headers, b.SyncPattern, b.SyncDistance, voiceSync)
		}
		emb, ok, err := rx.Receive(&b, nil)
		if b.VoiceBurst != wantPos {
			t.Fatalf("voice burst %d at %v, want %v", i-headers, b.VoiceBurst, wantPos)
		}
		if b.VoiceBurst == enums.VoiceBurstE {
			want := lc
			want.BurstDataType = enums.DataTypeVoiceLCHeader
			if err != nil || !ok || !reflect.DeepEqual(withoutFEC(&emb), withoutFEC(&want)) {
				t.Fatalf("embedded LC %s, %v, %v, want %s", emb.String(), ok, err, want.String())
			}
			lcs++
		} else if ok || err != nil {
			t.Fatalf("voice burst %d at %v gives an LC: %v, %v", i-headers, b.VoiceBurst, ok, err)
		}
		fr, res := b.AMBEFrames()
		if !clean(res) {
			t.Fatalf("voice burst %d decodes with %+v", i-headers, res)
		}
		got = append(got, fr[:]...)
	}
	if lcs != (n+17)/18 || len(got) != 18*lcs {
		t.Fatalf("%d frames gave %d embedded LCs and %d frames", n, lcs, len(got))
	}
	for i, fr := range got {
		want := silence
		if i < n {
			want = frames[i]
		}
		if fr != want {
			t.Fatalf("frame %d = %x, want %x", i, fr, want)
		}
	}
}

func FuzzVoiceTransmitter(f *testing.F) {
	for _, s := range transmitterSeeds() {
		f.Add(s.lc, s.cc, s.src, s.frames)
	}
	f.Fuzz(checkVoiceTransmitter)
}
