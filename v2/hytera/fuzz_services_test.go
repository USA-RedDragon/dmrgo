package hytera_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera"
)

type appender interface {
	AppendBinary(dst []byte) ([]byte, error)
}

func checkRoundTrip[T any, P interface {
	*T
	appender
}](t *testing.T, name string, decode func([]byte) (T, error), data []byte) {
	t.Helper()
	v, err := decode(data)
	if err != nil {
		if !errors.Is(err, hytera.ErrShort) && !errors.Is(err, hytera.ErrLength) && !errors.Is(err, hytera.ErrSignature) && !errors.Is(err, hytera.ErrText) {
			t.Fatalf("%s(%x): unexpected error %v", name, data, err)
		}
		return
	}
	out, err := P(&v).AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Fatalf("%s(%x) encodes to %x, %v", name, data, out, err)
	}
}

func gpsSeed(prefix ...byte) []byte {
	return append(prefix, gps...)
}

func FuzzDecodeLPEmergency(f *testing.F) {
	lpEmergencySeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeLPEmergency)
}

func lpEmergencySeeds(add func(data []byte)) {
	add(gpsSeed(10, 0, 0, 80, 1))
	add([]byte{10, 0, 0, 80})
}

func fuzzDecodeLPEmergency(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeLPEmergency", hytera.DecodeLPEmergency, data)
}

func FuzzDecodeLPReport(f *testing.F) {
	lpReportSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeLPReport)
}

func lpReportSeeds(add func(data []byte)) {
	add(gpsSeed(0, 0, 0, 7, 10, 0, 0, 80))
	add([]byte{0, 0, 0, 7, 10, 0, 0})
}

func fuzzDecodeLPReport(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeLPReport", hytera.DecodeLPReport, data)
}

func FuzzDecodeLPRequest(f *testing.F) {
	lpRequestSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeLPRequest)
}

func lpRequestSeeds(add func(data []byte)) {
	add([]byte{0, 0, 0, 7, 10, 0, 0, 80})
	add([]byte{0, 0, 0, 7, 10, 0, 0, 80, 0})
}

func fuzzDecodeLPRequest(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeLPRequest", hytera.DecodeLPRequest, data)
}

func FuzzDecodeRRSAnswer(f *testing.F) {
	rrsAnswerSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeRRSAnswer)
}

func rrsAnswerSeeds(add func(data []byte)) {
	add([]byte{10, 0, 0, 80, 0, 0, 0, 0x0e, 0x10})
	add([]byte{10, 0, 0, 80, 1})
}

func fuzzDecodeRRSAnswer(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeRRSAnswer", hytera.DecodeRRSAnswer, data)
}

func FuzzDecodeRRSCheckAnswer(f *testing.F) {
	rrsCheckAnswerSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeRRSCheckAnswer)
}

func rrsCheckAnswerSeeds(add func(data []byte)) {
	add([]byte{10, 0, 0, 80, 0})
	add([]byte{10, 0, 0, 80})
}

func fuzzDecodeRRSCheckAnswer(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeRRSCheckAnswer", hytera.DecodeRRSCheckAnswer, data)
}

func FuzzDecodeRRSRadio(f *testing.F) {
	rrsRadioSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeRRSRadio)
}

func rrsRadioSeeds(add func(data []byte)) {
	add([]byte{10, 0, 0, 80})
	add([]byte{10, 0, 0, 80, 0})
}

func fuzzDecodeRRSRadio(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeRRSRadio", hytera.DecodeRRSRadio, data)
}

func FuzzDecodeTMPAnswer(f *testing.F) {
	tmpAnswerSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeTMPAnswer)
}

func tmpAnswerSeeds(add func(data []byte)) {
	add([]byte{0, 0, 0, 1, 10, 0, 0, 2, 10, 0, 0, 1, 4})
	add([]byte{0, 0, 0, 1, 10, 0, 0, 2, 10, 0, 0, 1})
}

func fuzzDecodeTMPAnswer(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeTMPAnswer", hytera.DecodeTMPAnswer, data)
}

func FuzzDecodeTMPFlags(f *testing.F) {
	tmpFlagsSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeTMPFlags)
}

func tmpFlagsSeeds(add func(data []byte)) {
	add([]byte{0x00})
	add([]byte{0xc5})
	add([]byte{0x80, 0x00})
}

func fuzzDecodeTMPFlags(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeTMPFlags", hytera.DecodeTMPFlags, data)
}

func FuzzDecodeTMPGroupAnswer(f *testing.F) {
	tmpGroupAnswerSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeTMPGroupAnswer)
}

func tmpGroupAnswerSeeds(add func(data []byte)) {
	add([]byte{0, 0, 0, 2, 225, 0, 0, 9, 0})
	add([]byte{0, 0, 0, 2, 225, 0, 0, 9})
}

func fuzzDecodeTMPGroupAnswer(t *testing.T, data []byte) {
	checkRoundTrip(t, "DecodeTMPGroupAnswer", hytera.DecodeTMPGroupAnswer, data)
}
