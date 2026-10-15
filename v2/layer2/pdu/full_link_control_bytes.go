package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// FullLinkControlFromBytes decodes the 72 LC bits lc, without RS(12,9)
// parity, as a FullLinkControl. FEC and BurstDataType are left at their
// zero values; set BurstDataType before Encode. It returns an error when
// the decoded LC does not encode back to lc, so that no bit of lc is lost.
func FullLinkControlFromBytes(lc [9]byte) (FullLinkControl, error) {
	cw, _ := LCBytes(lc).Codeword(enums.DataTypeVoiceLCHeader)
	f := DecodeFullLinkControl(bit.Unpack96(cw), enums.DataTypeVoiceLCHeader)
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

// Codeword returns the 12-octet on-air codeword of f in a burst of
// f.BurstDataType, as LCBytes.Codeword gives it. It returns the errors that
// Encode returns for the fields, and an error unless BurstDataType is
// DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
func (f *FullLinkControl) Codeword() ([12]byte, error) {
	lc, err := f.Bytes()
	if err != nil {
		return [12]byte{}, err
	}
	return LCBytes(lc).Codeword(f.BurstDataType)
}

// Bytes returns the 72 LC bits of f packed most significant bit first,
// without RS(12,9) parity. It ignores BurstDataType and returns the errors
// that Encode returns for the fields.
func (f *FullLinkControl) Bytes() ([9]byte, error) {
	c := *f
	c.BurstDataType = enums.DataTypeVoiceLCHeader
	bits, err := c.Encode()
	if err != nil {
		return [9]byte{}, err
	}
	var lc [9]byte
	copy(lc[:], bit.Pack(bits[:72]))
	return lc, nil
}
