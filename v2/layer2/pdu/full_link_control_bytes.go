package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// DecodeFullLinkControlBytes decodes the 72 LC bits lc, without RS(12,9)
// parity, as a FullLinkControl. FEC and BurstDataType are left at their
// zero values; set BurstDataType before Encode. It returns an error when
// the decoded LC does not encode back to lc, so that no bit of lc is lost.
func DecodeFullLinkControlBytes(lc [9]byte) (FullLinkControl, error) {
	cw, _ := LCBytes(lc).Codeword(enums.DataTypeVoiceLCHeader)
	f := decodeFullLinkControlBytes(cw, enums.DataTypeVoiceLCHeader)
	f.BurstDataType = 0
	f.FEC = fec.Result{}
	got, err := f.Bytes()
	if err != nil {
		return FullLinkControl{}, err
	}
	if got != lc {
		return FullLinkControl{}, fmt.Errorf("full LC %x encodes back as %x", lc, got)
	}
	return f, nil
}

// DecodeFullLinkControlBytesInto decodes lc into dst as
// DecodeFullLinkControlBytes does, with the payload in p as
// DecodeFullLinkControlInto has it. It returns the errors of
// DecodeFullLinkControlBytes and sets dst to the zero value on an error.
func DecodeFullLinkControlBytesInto(dst *FullLinkControl, p *FullLinkControlPayloads, lc [9]byte) error {
	cw, _ := LCBytes(lc).Codeword(enums.DataTypeVoiceLCHeader)
	decodeFullLinkControlBytesInto(dst, p, cw, enums.DataTypeVoiceLCHeader)
	dst.BurstDataType = 0
	dst.FEC = fec.Result{}
	got, err := dst.Bytes()
	if err == nil && got != lc {
		err = fmt.Errorf("full LC %x encodes back as %x", lc, got)
	}
	if err != nil {
		*dst = FullLinkControl{}
		return err
	}
	return nil
}

// Codeword returns the 12-octet on-air codeword of s in a burst of
// s.BurstDataType, as LCBytes.Codeword gives it. It returns the errors that
// Encode returns for the fields, and an error unless BurstDataType is
// DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
func (s *FullLinkControl) Codeword() ([12]byte, error) {
	lc, err := s.Bytes()
	if err != nil {
		return [12]byte{}, err
	}
	return LCBytes(lc).Codeword(s.BurstDataType)
}

// Bytes returns the 72 LC bits of s packed most significant bit first,
// without RS(12,9) parity. It ignores BurstDataType and returns the errors
// that Encode returns for the fields.
func (s *FullLinkControl) Bytes() ([9]byte, error) {
	c := *s
	c.BurstDataType = enums.DataTypeVoiceLCHeader
	cw, err := c.encodeBytes()
	if err != nil {
		return [9]byte{}, err
	}
	return [9]byte(cw[:9]), nil
}
