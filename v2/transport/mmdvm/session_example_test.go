package mmdvm_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

// Example opens a session with a modem as MMDVMHost does: it requests the
// version, sends the config of a DMR repeater on color code 7 and switches
// to DMR mode. The modem then sends a burst that it received on slot 2.
// A simulated modem in memory stands in for the serial port.
func Example() {
	var fromModem []byte
	toModem := func(c mmdvm.Command, payload []byte) {
		frame, err := mmdvm.AppendFrame(nil, c, payload)
		if err != nil {
			fmt.Println(err)
			return
		}
		fromModem = modem(fromModem, frame)
	}

	toModem(mmdvm.CommandGetVersion, nil)
	cfg := mmdvm.ConfigV2{DMR: true, DMRColorCode: 7, State: mmdvm.ModeIdle, RXLevel: 50, DMRTXLevel: 50}
	payload, _ := cfg.AppendBinary(nil)
	toModem(mmdvm.CommandSetConfig, payload)
	mode := mmdvm.SetMode{Mode: mmdvm.ModeDMR}
	payload, _ = mode.AppendBinary(nil)
	toModem(mmdvm.CommandSetMode, payload)

	for len(fromModem) > 0 {
		f, n, r := mmdvm.Next(fromModem)
		if r == mmdvm.NextMore {
			break
		}
		fromModem = fromModem[n:]
		if r == mmdvm.NextSkip {
			continue
		}
		switch f.Command {
		case mmdvm.CommandGetVersion:
			v, _ := mmdvm.DecodeVersion(f.Payload)
			fmt.Printf("version: protocol %d, DMR %v, %s\n", v.Protocol, v.Capabilities.Has(mmdvm.CapDMR), v.Description)
		case mmdvm.CommandAck:
			a, _ := mmdvm.DecodeAck(f.Payload)
			fmt.Println("ack:", a.Command)
		case mmdvm.CommandDMRData1, mmdvm.CommandDMRData2:
			d, err := mmdvm.DecodeDMRData(f)
			if err != nil {
				fmt.Println(err)
				continue
			}
			h := d.Hint()
			b, err := layer2.NewBurstFromBytes(d.Burst)
			if err != nil {
				fmt.Println(err)
				continue
			}
			fmt.Println("burst:", h.Slot, h.Kind, h.DataType, "color code", b.SlotType.ColorCode)
		default:
			fmt.Println("frame:", f.Command)
		}
	}
	// Output:
	// version: protocol 2, DMR true, MMDVM simulated
	// ack: SetConfig
	// ack: SetMode
	// burst: TS2 Data Voice LC Header color code 7
}

func modem(out, frame []byte) []byte {
	f, _, r := mmdvm.Next(frame)
	if r != mmdvm.NextFrame {
		return out
	}
	switch f.Command {
	case mmdvm.CommandGetVersion:
		v := mmdvm.Version{Protocol: 2, Capabilities: mmdvm.CapDMR, CPU: mmdvm.CPUST, Description: []byte("MMDVM simulated")}
		payload, _ := v.AppendBinary(nil)
		out, _ = mmdvm.AppendFrame(out, mmdvm.CommandGetVersion, payload)
	case mmdvm.CommandSetConfig, mmdvm.CommandSetMode:
		a := mmdvm.Ack{Command: f.Command}
		payload, _ := a.AppendBinary(nil)
		out, _ = mmdvm.AppendFrame(out, mmdvm.CommandAck, payload)
		if f.Command == mmdvm.CommandSetMode {
			raw, _ := hex.DecodeString("20580b4015ec0f6871000d005c4d5d7f77fd757361a416203d5235242181850277")
			d := mmdvm.DMRData{Slot: enums.Slot2, Control: mmdvm.ControlDataSync | mmdvm.Control(enums.DataTypeVoiceLCHeader)}
			copy(d.Burst[:], raw)
			payload, _ = d.AppendBinary(nil)
			out, _ = mmdvm.AppendFrame(out, d.Command(), payload)
		}
	default:
	}
	return out
}
