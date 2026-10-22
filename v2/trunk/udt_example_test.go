package trunk_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

// ExampleAppended codes text in the ISO 7 bit format of a UDT and reads it
// back.
func ExampleAppended() {
	var a trunk.Appended
	_ = a.SetText(enums.UDTISO7, []byte("ON SCENE"))
	text, _ := a.Text(nil)
	fmt.Println(a.Format, a.Blocks, "block", a.PadNibble, "pad nibbles:", string(text))
	_ = a.SetDigits([]byte("5551234#"))
	digits, _ := a.Digits(nil)
	fmt.Printf("%s % x\n", digits, a.Data[:5])
	// Output:
	// ISO 7 bit 1 block 6 pad nibbles: ON SCENE
	// 5551234# 55 51 23 4b ff
}

// ExampleMS_SendShortData sends UDT short data: the MS asks by random
// access, uploads the data when the TSCC asks with a C_AHOY from SDMI, and
// reports the final acknowledgement.
func ExampleMS_SendShortData() {
	code, _ := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 1, Site: 2}.Pack()
	bs := func(c *pdu.CSBK) layer2.RawBurst {
		c.LastBlock = true
		raw, _ := layer2.BuildBurst(c, 1, layer2.SyncSourceBS)
		return raw
	}
	aloha := bs(&pdu.CSBK{Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{SysIdentCode: code}})
	ahoy := bs(&pdu.CSBK{Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{ServiceKind: uint8(enums.ServiceKindIndividualShortData),
		TargetAddress: 3120001, SourceAddress: constants.GatewaySDMI}})
	done := bs(&pdu.CSBK{Opcode: pdu.CSBKAckOutbound, AckOutboundPDU: &pdu.AckOutboundPDU{
		ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: 3120001}})

	ms, _ := trunk.NewMS(&trunk.MSConfig{Address: 3120001, NoRegistration: true})
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	free := [3]byte{}
	now := time.Duration(0)
	var a trunk.Appended
	_ = a.SetText(enums.UDTISO8, []byte("meet at gate 4"))
	for i, raw := range []layer2.RawBurst{aloha, aloha, ahoy, done} {
		if i == 1 {
			_ = ms.SendShortData(now, false, 3120002, &a, layer3.ShortDataServiceOptions{})
		}
		out = ms.Burst(now, &raw, layer2.Hint{Slot: enums.Slot1}, &free, out[:0])
		for _, o := range out {
			switch o.Kind {
			case trunk.OutputBurst:
				fmt.Println("send at", o.Due)
			case trunk.OutputDelivered:
				fmt.Println("delivered:", o.Ack)
			default:
				fmt.Println(o.Kind)
			}
		}
		now += 60 * time.Millisecond
	}
	// Output:
	// Site
	// send at 120ms
	// send at 180ms
	// send at 240ms
	// send at 300ms
	// delivered: MS_Accepted
}
