package pdu_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ExampleLCBytes_Codeword encodes a Full LC codeword, checks its
// parity and corrects one bit error.
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

// ExampleDecodeCSBK encodes a preamble CSBK and decodes it back.
func ExampleDecodeCSBK() {
	in := pdu.CSBK{LastBlock: true}
	err := in.SetPreamble(pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2, TargetAddress: 91, SourceAddress: 3120000})
	if err != nil {
		fmt.Println(err)
		return
	}
	bits, err := in.Encode()
	if err != nil {
		fmt.Println(err)
		return
	}
	out := pdu.DecodeCSBK(bits)
	pre, _ := out.AsPreamble()
	fmt.Println(out.FEC.Uncorrectable, pre.Group, pre.TargetAddress, pre.SourceAddress)
	// Output:
	// false true 91 3120000
}

// ExampleBroadcastPDU_AsLocalTime reads the local time that a TSCC
// broadcasts in C_BCAST Local_Time.
func ExampleBroadcastPDU_AsLocalTime() {
	in := pdu.CSBK{LastBlock: true}
	err := in.SetBroadcast(pdu.BroadcastPDU{
		AnnouncementType: enums.AnnouncementLocalTime, BroadcastParms1: 21<<9 | 10<<5 | 5<<1 | 1,
		Backoff: 1, SysIdentCode: 0x4015, BroadcastParms2: 11<<19 | 49<<13 | 50<<7 | 4<<4,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	bits, err := in.Encode()
	if err != nil {
		fmt.Println(err)
		return
	}
	out := pdu.DecodeCSBK(bits)
	bcast, ok := out.AsBroadcast()
	if !ok {
		fmt.Println("not a C_BCAST")
		return
	}
	lt, ok := bcast.AsLocalTime()
	sign := "+"
	if lt.UTCOffsetLags {
		sign = "-"
	}
	fmt.Printf("%v %02d/%02d %02d:%02d:%02d UTC%s%d\n", ok, lt.Month, lt.Day, lt.Hours, lt.Minutes, lt.Seconds, sign, lt.UTCOffset)
	// Output:
	// true 10/21 11:49:50 UTC-5
}
