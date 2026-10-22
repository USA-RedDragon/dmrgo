package pdu

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// DecodeFullLinkControlBytes decodes the 72 LC bits lc, without RS(12,9)
// parity, as a FullLinkControl. FEC and BurstDataType are left at their
// zero values; set BurstDataType before Encode. It returns an error when
// the decoded LC does not encode back to lc, so that no bit of lc is lost.
func DecodeFullLinkControlBytes(lc [9]byte) (FullLinkControl, error) {
	cw, _ := LCBytes(lc).Codeword(enums.DataTypeVoiceLCHeader)
	f := DecodeFullLinkControl(cw, enums.DataTypeVoiceLCHeader)
	f.BurstDataType = 0
	f.FEC = fec.Result{}
	got, err := f.Bytes()
	if err != nil {
		return FullLinkControl{}, err
	}
	if got != lc {
		return FullLinkControl{}, errRoundTrip
	}
	return f, nil
}

// Bytes returns the 72 LC bits of s packed most significant bit first,
// without RS(12,9) parity. It ignores BurstDataType and returns the errors
// that Encode returns for the fields, and an error for a nil s.
func (s *FullLinkControl) Bytes() ([9]byte, error) {
	if s == nil {
		return [9]byte{}, errNil
	}
	c := *s
	c.BurstDataType = enums.DataTypeVoiceLCHeader
	cw, err := c.Encode()
	if err != nil {
		return [9]byte{}, err
	}
	return [9]byte(cw[:9]), nil
}

var errRoundTrip = errors.New("pdu: the full LC does not encode back to the same octets")
