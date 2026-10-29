//go:build !tinygo

package hytera_test

import (
	"fmt"
	"net"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// ExampleFrame_SetCallHint sends a group call from a voice.Transmitter as
// frames of the DMR service over UDP on the loopback interface, and
// follows the call with a voice.Receiver on the other side.
func ExampleFrame_SetCallHint() {
	if err := relay(); err != nil {
		fmt.Println(err)
	}
	// Output:
	// Voice LC Header on TS1 from 3191868 to 9 group true
	// Terminator with LC after 6 voice bursts: Terminator
}

func relay() error {
	loopback := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	rxConn, err := net.ListenUDP("udp4", loopback)
	if err != nil {
		return err
	}
	defer rxConn.Close()
	txConn, err := net.ListenUDP("udp4", loopback)
	if err != nil {
		return err
	}
	defer txConn.Close()

	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceBS,
		Silence: vocoder.AMBESilenceSlot(), Access: enums.AccessImpolite, Queue: voice.MaxQueue})
	if err != nil {
		return err
	}
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		return err
	}
	if err := tx.Start(0, voice.Call{Group: true, Target: 9, Source: 3191868}, nil); err != nil {
		return err
	}
	for range 18 {
		if err := tx.Frame(vocoder.AMBESilence()); err != nil {
			return err
		}
	}
	tx.Stop(0)

	var f hytera.Frame
	out := make([]voice.Output, 0, voice.MaxOutputs)
	events := make([]voice.Event, 0, voice.MaxEvents)
	packet := make([]byte, 0, hytera.FrameLen)
	buf := make([]byte, 1500)
	voiceBursts := 0
	for now := time.Duration(0); tx.State() != voice.TransmitterIdle; now += 60 * time.Millisecond {
		for _, o := range tx.Tick(now, out[:0]) {
			if o.Kind != voice.OutputBurst {
				continue
			}
			f.Burst, f.ColorCode, f.PacketType = o.Burst, 1, 0x01
			if o.Hint.DataType == enums.DataTypeTerminatorWithLC {
				f.PacketType = 0x03
			}
			if err := f.SetCallHint(o.Hint, o.Call); err != nil {
				return err
			}
			if packet, err = f.AppendBinary(packet[:0]); err != nil {
				return err
			}
			f.Seq++
			if _, err := txConn.WriteTo(packet, rxConn.LocalAddr()); err != nil {
				return err
			}

			_ = rxConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, _, err := rxConn.ReadFrom(buf)
			if err != nil {
				return err
			}
			got, err := hytera.DecodeFrame(buf[:n])
			if err != nil || !got.HasBurst() {
				continue
			}
			for _, e := range rx.Burst(o.Due, &got.Burst, got.Hint(o.Due), events[:0]) {
				switch e.Kind {
				case voice.EventCallStart:
					group, target, source := got.Call()
					fmt.Println(got.SlotType, "on", got.Slot, "from", source, "to", target, "group", group)
				case voice.EventVoice:
					voiceBursts++
				case voice.EventCallEnd:
					fmt.Println(got.SlotType, "after", voiceBursts, "voice bursts:", e.Reason)
				default:
				}
			}
		}
	}
	return nil
}
