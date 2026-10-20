package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const weakCapture = "testdata/xpr8400_bs_voice_weak.cu8"

type echoCodec struct {
	slots [][9]byte
}

func (c *echoCodec) Close() error            { return nil }
func (c *echoCodec) FrameSamples() int       { return 160 }
func (c *echoCodec) Silence() [9]byte        { return vocoder.AMBESilenceSlot() }
func (c *echoCodec) FID() enums.FeatureSetID { return enums.FIDStandardized }

func (c *echoCodec) Encode([]int16) ([9]byte, error) { return [9]byte{}, errors.New("not used") }

func (c *echoCodec) Decode(slot [9]byte) ([]int16, error) {
	c.slots = append(c.slots, slot)
	pcm := dvsisim.Echo(slot)
	return pcm[:], nil
}

func weakSlots(t *testing.T) [][9]byte {
	t.Helper()
	iq, err := os.ReadFile(weakCapture)
	if err != nil {
		t.Fatal(err)
	}
	rec := &echoCodec{}
	rep := NewReporter(io.Discard)
	rep.audio = &audioOut{codec: rec, w: io.Discard}
	for _, r := range demodulate(t, rftx.FromRTL(iq), 256e3).Bursts {
		rep.Burst(r)
	}
	if len(rec.slots) != 18 {
		t.Fatalf("%d voice slots in the weak capture, want 18", len(rec.slots))
	}
	return rec.slots
}

func checkPCM(t *testing.T, path string, slots [][9]byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want []byte
	for _, s := range slots {
		for _, v := range dvsisim.Echo(s) {
			want = binary.LittleEndian.AppendUint16(want, uint16(int(v)&0xFFFF))
		}
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("%s: %d bytes of PCM, want the %d bytes of the echo of %d frames", path, len(b), len(want), len(slots))
	}
}

func runWeak(t *testing.T, spec string) (string, string) {
	t.Helper()
	pcm := filepath.Join(t.TempDir(), "voice.pcm")
	var out bytes.Buffer
	err := run([]string{"rx", "-i", weakCapture, "-rate", "256000", "-ambe", spec, "-pcm", pcm}, &out)
	if err != nil {
		t.Fatalf("-ambe %s: %v\n%s", spec, err, out.String())
	}
	if !strings.Contains(out.String(), "audio: 18 frames, 0.36 s") {
		t.Errorf("-ambe %s: no audio summary in\n%s", spec, out.String())
	}
	return pcm, out.String()
}

func TestAMBESpecErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ args, want string }{
		{"-ambe software:x", "takes no argument"},
		{"-ambe dvsi", "want software"},
		{"-ambe serial:", "no device"},
		{"-ambe serial:/dev/null@fast", "baud"},
		{"-ambe serial:/dev/null@1234", "baud 1234"},
		{"-ambe serial:/nonexistent/tty", "no such file"},
		{"-pcm x.pcm", "-pcm needs -ambe"},
	} {
		args := append([]string{"rx", "-i", weakCapture, "-rate", "256000"}, strings.Fields(tt.args)...)
		if err := run(args, io.Discard); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want an error with %q", tt.args, err, tt.want)
		}
	}
}

func TestAMBESoftware(t *testing.T) {
	t.Parallel()

	slots := weakSlots(t)
	args := []string{"rx", "-i", weakCapture, "-rate", "256000", "-ambe", "software"}
	if err := run(args, io.Discard); err == nil || !strings.Contains(err.Error(), `no codec registered as "ambe"`) {
		t.Errorf("-ambe software before the codec is registered: %v", err)
	}
	rec := &echoCodec{}
	if err := vocoder.Register(softwareCodec, func() (vocoder.Codec, error) { return rec, nil }); err != nil {
		t.Fatal(err)
	}
	pcm, out := runWeak(t, "software")
	checkPCM(t, pcm, slots)
	if !strings.Contains(out, "ambe: software codec ambe") || len(rec.slots) != len(slots) {
		t.Errorf("%d frames decoded\n%s", len(rec.slots), out)
	}
}

func serveAMBEServer(t *testing.T, d *dvsisim.Device) string {
	t.Helper()
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		out := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = d.Write(buf[:n])
			for {
				k, err := d.Read(out)
				if err != nil || k == 0 {
					break
				}
				if _, err := pc.WriteTo(out[:k], from); err != nil {
					return
				}
			}
		}
	}()
	return pc.LocalAddr().String()
}

func TestAMBEServer(t *testing.T) {
	t.Parallel()

	slots := weakSlots(t)
	d := dvsisim.New(nil, dvsisim.Options{Datagram: true, Echo: true})
	addr := serveAMBEServer(t, d)
	pcm, out := runWeak(t, "ambeserver:"+addr)
	checkPCM(t, pcm, slots)
	if !strings.Contains(out, "ambe: AMBE3000R V120.") || !strings.Contains(out, "on AMBEserver "+addr) {
		t.Errorf("no device line in\n%s", out)
	}
	if counts, errs := d.Snapshot(); len(errs) > 0 || counts.Decode != len(slots) {
		t.Errorf("device: %d decodes, errors %q", counts.Decode, errs)
	}
}

func TestAMBEServerSilent(t *testing.T) {
	t.Parallel()

	var lc net.ListenConfig
	pc, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	start := time.Now()
	err = run([]string{"rx", "-i", weakCapture, "-rate", "256000", "-ambe", "ambeserver:" + pc.LocalAddr().String()}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(start) > 10*time.Second {
		t.Errorf("silent AMBEserver: %v after %v", err, time.Since(start))
	}
}
