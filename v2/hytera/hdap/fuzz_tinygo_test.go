//go:build tinygo

package hdap_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	decodeSeeds(func(data []byte) { fuzzDecode(t, data) })
	lpEmergencySeeds(func(data []byte) { fuzzDecodeLPEmergency(t, data) })
	lpReportSeeds(func(data []byte) { fuzzDecodeLPReport(t, data) })
	lpRequestSeeds(func(data []byte) { fuzzDecodeLPRequest(t, data) })
	rrsAnswerSeeds(func(data []byte) { fuzzDecodeRRSAnswer(t, data) })
	rrsCheckAnswerSeeds(func(data []byte) { fuzzDecodeRRSCheckAnswer(t, data) })
	rrsRadioSeeds(func(data []byte) { fuzzDecodeRRSRadio(t, data) })
	tmpAnswerSeeds(func(data []byte) { fuzzDecodeTMPAnswer(t, data) })
	tmpFlagsSeeds(func(data []byte) { fuzzDecodeTMPFlags(t, data) })
	tmpGroupAnswerSeeds(func(data []byte) { fuzzDecodeTMPGroupAnswer(t, data) })
}
