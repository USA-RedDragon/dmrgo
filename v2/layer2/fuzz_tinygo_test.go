//go:build tinygo

package layer2_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, r := range captureFrames(t) {
		checkBurstDecode(t, r[:])
	}
	for _, r := range fuzzCorpus(t, "FuzzBurstDecode") {
		checkBurstDecode(t, r[:])
	}
	for _, s := range decodeWithSeeds(t) {
		checkBurstDecodeWith(t, s.raw, s.kind, s.pos, s.dt, s.thr, s.cc)
	}
	fs := newRawFields()
	for _, s := range rawFieldSeeds(t) {
		fs.check(t, s.raw, s.v)
	}
	for _, s := range dataSeeds(t) {
		checkBuilders(t, s.dt, s.cc, s.src, s.payload)
		checkMMDVMHostSeal(t, s.dt, s.cc, s.src, s.payload)
	}
	for _, s := range voiceBurstSeeds() {
		checkBuildVoiceBurst(t, s.frames, s.pos, s.emb, s.embedded, s.src)
	}
	for _, s := range transmitterSeeds() {
		checkVoiceTransmitter(t, s.lc, s.cc, s.src, s.frames)
	}
	for _, s := range cachSeeds {
		checkCACH(t, s)
	}
	for _, s := range shortLCSeeds {
		checkShortLCAssembler(t, s.seq, s.slc, s.at, s.tact)
	}
	for _, s := range embeddedLCSeeds() {
		checkEmbeddedLCAssembler(t, s.seq, s.lc, s.at)
	}
	for _, s := range embeddedLCCodeSeeds() {
		checkEmbeddedLCCode(t, s.lc, s.flc, s.flips)
	}
	for _, s := range rcSeeds {
		checkEmbeddedRC(t, s.field, s.cmd)
	}
}
