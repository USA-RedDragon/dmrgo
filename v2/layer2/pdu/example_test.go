package pdu_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func ExampleLCBytes_Codeword() {
	lc := pdu.LCBytes{0x00, 0x00, 0x00, 0x00, 0x00, 0x5b, 0x00, 0x30, 0x39}
	cw, err := lc.Codeword(enums.DataTypeVoiceLCHeader)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%x\n", cw)
	fmt.Println(pdu.FullLCParityOK(cw, enums.DataTypeVoiceLCHeader))
	fmt.Println(pdu.FullLCParityOK(cw, enums.DataTypeTerminatorWithLC))

	cw[4] ^= 0x10
	got, res, _ := pdu.DecodeFullLCCodeword(cw, enums.DataTypeVoiceLCHeader)
	fmt.Println(got == [9]byte(lc), res.ErrorsCorrected)
	// Output:
	// 00000000005b003039b881b1
	// true
	// false
	// true 1
}

func ExampleDecodeCSBK() {
	in := pdu.CSBK{
		LastBlock:   true,
		Opcode:      pdu.CSBKPreamble,
		PreamblePDU: &pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2, TargetAddress: 91, SourceAddress: 3120000},
	}
	bits, err := in.Encode()
	if err != nil {
		fmt.Println(err)
		return
	}
	out := pdu.DecodeCSBK(bits)
	fmt.Println(out.FEC.Uncorrectable, out.PreamblePDU.Group, out.PreamblePDU.TargetAddress, out.PreamblePDU.SourceAddress)
	// Output:
	// false true 91 3120000
}
