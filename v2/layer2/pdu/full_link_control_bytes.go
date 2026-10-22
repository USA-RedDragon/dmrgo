package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// DecodeFullLinkControlBytes decodes the 72 LC bits lc, without RS(12,9)
// parity, as a FullLinkControl. FEC and BurstDataType are left at their
// zero values; set BurstDataType before Encode. Every bit of lc is kept, so
// Bytes returns lc.
func DecodeFullLinkControlBytes(lc [9]byte) FullLinkControl {
	cw, _ := LCBytes(lc).Codeword(enums.DataTypeVoiceLCHeader)
	f := DecodeFullLinkControl(cw, enums.DataTypeVoiceLCHeader)
	f.BurstDataType = 0
	f.FEC = fec.Result{}
	return f
}

// Bytes returns the 72 LC bits of s packed most significant bit first,
// without RS(12,9) parity. It ignores BurstDataType and returns the errors
// that Encode returns for the fields, and dmrgo.ErrNilReceiver for a nil s.
func (s *FullLinkControl) Bytes() ([9]byte, error) {
	if s == nil {
		return [9]byte{}, dmrgo.ErrNilReceiver
	}
	c := *s
	c.BurstDataType = enums.DataTypeVoiceLCHeader
	cw, err := c.Encode()
	if err != nil {
		return [9]byte{}, err
	}
	return [9]byte(cw[:9]), nil
}
