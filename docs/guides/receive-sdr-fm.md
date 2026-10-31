# Receive with an SDR or an FM radio

This guide shows how to decode DMR bursts from the I/Q samples of a software radio or from the discriminator audio of an FM radio, with the dmrrf tool or with package rf in your own program.

## Goal

Turn received samples into DMR bursts, follow the calls on both TDMA channels, and give the bursts to the voice and data machines.

## What you need

- Go and a checkout of dmrgo. The steps run offline on a file that dmrrf writes.
- For a live signal, one of:
  - a software radio, such as an RTL-SDR, a HackRF or a SoapySDR radio, with its capture tool;
  - an FM radio or modem with a discriminator output, through a sound card or the ADC of a microcontroller.
- The package [rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf).

## Steps

### 1. Decode a file with dmrrf

[dmrrf](../../v2/cmd/dmrrf/README.md) reads I/Q or audio from a file or a pipe and prints each burst that it decodes. Build it, write a test transmission with `dmrrf tx`, and decode it with `dmrrf rx`:

```sh
cd v2
go build -o dmrrf ./cmd/dmrrf
./dmrrf tx -seq voice -slot 0 -rate 1000000 -o voice.cs16
./dmrrf rx -i voice.cs16 -rate 1000000
```

`dmrrf rx` prints a line for each burst and each decoded field, and a summary with the carrier offset and the deviation at the end. The README of dmrrf gives the sample formats of common software radios, the commands for an RTL-SDR, a HackRF, a SoapySDR radio, a USRP and GNU Radio, and the flags for frequency correction and voice output.

### 2. Decode I/Q in your program

[rf.NewReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewReceiver) makes a [Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Receiver) for the sample rate and the format of your radio: [IQFormatCU8](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#IQFormat) as `rtl_sdr` writes it, `IQFormatCS8` as `hackrf_transfer` writes it, `IQFormatCS16` or `IQFormatCF32`. The `Tier` of the config sets the detection and its cost. Pass the samples to `Write` as you read them; the receiver calls `OnBursts` with the bursts that they complete. `Flush` ends the stream. With `OnBursts` nil, use `AppendBursts` and `AppendFlush` instead.

A [Monitor](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Monitor) follows the TDMA channels, as a scanner does. It finds the channel of each burst from the CACHs of its stream, decodes the burst and keeps the LC of the call on each channel. [Burst.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Burst.Hint) gives the `layer2.Hint` of a burst for the voice and data machines.

<!-- dmrgo:code file=v2/rf/example_test.go symbol=ExampleMonitor -->
```go
func ExampleMonitor() {
	built, err := rftx.Sequence("voice", rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, BS: 314926, Group: true, Wakeups: 1,
		Superframes: 2, BSSourced: true, Slot: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	fm, env := rftx.ModulateBS(built.Slots)
	iq := cf32(rftx.Baseband(fm, env, 800))
	var m rf.Monitor
	lb := layer2.NewBurst()
	voice := 0
	onBursts := func(bs []rf.Burst) {
		for i := range bs {
			r, err := m.Burst(lb, &bs[i])
			if err != nil || r.Hint.Slot != enums.Slot2 {
				continue
			}
			if lb.Kind == layer2.KindVoice {
				voice++
			}
			if r.End {
				dst, src, _ := r.LC.Addresses()
				fmt.Printf("call from %d to %d ended after %d voice bursts\n", src, dst, voice)
			}
		}
	}
	rx, err := rf.NewReceiver(&rf.RxConfig{Tier: rf.Tier0, Rate: 48000, Format: rf.IQFormatCF32, OnBursts: onBursts})
	if err != nil {
		fmt.Println(err)
		return
	}
	if _, err := rx.Write(iq); err != nil {
		fmt.Println(err)
		return
	}
	if err := rx.Flush(); err != nil {
		fmt.Println(err)
	}
}
```
<!-- /dmrgo:code -->

### 3. Decode discriminator audio

An FM radio gives the 4FSK signal at its discriminator output. [rf.NewAudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewAudioReceiver) makes an [AudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioReceiver) for the sample rate and the sample width of the audio. It has the same `Write`, `Flush` and `OnBursts` as the I/Q Receiver.

On a microcontroller, [AudioReceiver.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioReceiver.Init) sets up the receiver in Tier 0 in memory of the caller, without an allocation. The rxaudio example runs it as a hotspot board does, with a placeholder `read` in place of the ADC:

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=rx -->
```go
var (
	rx     rf.AudioReceiver
	buf    [256]int16
	bursts int
)
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=onBursts -->
```go
func onBursts(bs []rf.Burst) { bursts += len(bs) }
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=read -->
```go
func read(x []int16) {
	clear(x)
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=main -->
```go
func main() {
	if err := rx.Init(rf.AudioRxConfig{Rate: 9600, OnBursts: onBursts}); err != nil {
		return
	}
	for range 1000 {
		read(buf[:])
		if _, err := rx.Write(buf[:]); err != nil {
			return
		}
	}
	if err := rx.Flush(); err != nil {
		return
	}
	println("bursts:", bursts > 0)
}
```
<!-- /dmrgo:code -->

It builds with TinyGo and `-gc=none`, which fails the link of any program that allocates. The page [Microcontrollers](../targets/mcu.md) gives the build.

## Output

The Monitor follows a group call on TDMA channel 2 of a repeater:

<!-- dmrgo:output test=rf.ExampleMonitor -->
```text
call from 3191868 to 9 ended after 12 voice bursts
```
<!-- /dmrgo:output -->

The rxaudio example on a PC, with silence as its input:

<!-- dmrgo:output example=rxaudio -->
```text

```
<!-- /dmrgo:output -->

The configs of the two receivers:

<!-- dmrgo:config pkg=rf type=RxConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Tier` | `Tier` | `Tier0` (1) or `discDefaultTier` (2) | Tier selects the detection. The zero value TierDefault is Tier1 for NewReceiver, and Tier0 for Init and under TinyGo. |
| `Rate` | `int` | `defaultRxRate` (2400000) | Rate is the sample rate of the I/Q, in Hz. 0 means 2400000. Tier0 to Tier3 take a Rate from 48000 to 1e9, and Tier0 and Tier1 also one from 24000, which runs their receiver with a complex front end in place of the FM discriminator, with about 2 KB of state for Tier0. Tier0 also takes 9600 and 12000, for I/Q that a radio chip filtered to the channel, with about 1.4 KB of state; Init sets up only these. |
| `Format` | `IQFormat` | `IQFormatCS16` (3) | Format is the layout of the I/Q samples. 0 means IQFormatCS16. |
| `OffsetHz` | `float64` | not machine-readable; see the description | OffsetHz is the frequency of the carrier relative to the center of the I/Q, in Hz, when the oscillator of the radio is known to be off, as for an RTL-SDR without ppm correction (rtl_fm -p): the offset that OffsetHz of a Receiver reported for that radio. The receiver tunes there before it searches 6.25 kHz around it for the carrier. It must be under 24000 in size, or under Rate/2 for a Rate under 48000. 0 means a centered carrier. |
| `OnBursts` | `func([]Burst)` | zero value | OnBursts, when not nil, selects stream mode: Write and Flush hand the bursts to it. Each call carries the bursts that are ready at the same point of the stream, in storage of the receiver that is valid only during the call: usually one, and after a late lock the LookbackSlots bursts decoded again. When nil, the receiver is in append mode, for AppendBursts and AppendFlush. |
| `LookbackSlots` | `int` | `16` | LookbackSlots is the number of 30 ms slots before the first SYNC it locks to that the receiver keeps and decodes: 0 means 4 for Tier1, 8 for Tier2, and 16 for Tier3, and at most MaxLookbackSlots. Tier0 keeps none, and loses the start of a stream that it enters late. |
| `OnShortLC` | `func([]ShortLC)` | zero value | OnShortLC, when not nil, receives each Short LC that the CACH of the bursts carries and that passes its CRC-8, in stream and in append mode, in storage of the receiver that is valid only during the call: usually one. The receiver calls it as it outputs the burst of the last fragment, before that burst reaches OnBursts or the slice of AppendBursts. It decodes the fragments of the Decoded CACH as layer2.ShortLCAssembler does, and from Tier1 up decodes those that fail again from their soft values. Tier0 gives the hard decode only. Init and TinyGo builds take no OnShortLC and return a *dmrgo.ConfigError for it. |
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=rf type=AudioRxConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Tier` | `Tier` | `discDefaultTier` (2) | Tier is Tier0, Tier1, Tier2 or Tier3. The zero value TierDefault is Tier1 for NewAudioReceiver, and Tier0 for Init, which does not allocate the stages of the higher tiers, and under TinyGo, which has neither. Tier2 detects the bursts again coherently, on the carrier phase that it rebuilds from the samples, at about 5 times the CPU time of Tier1. Tier3 runs Tier2 and, beside it, a receiver with more coherent CPU time and four passes over each data burst. It keeps every burst of Tier2 and adds the bursts that Tier2 misses, the data bursts that pass a CRC where those of Tier2 fail, and the embedded LCs that pass where those of Tier2 fail, at about twice the CPU time and memory of Tier2, and 3 s of delay. |
| `Rate` | `int` | `2 * discRate` (48000) | Rate is the sample rate of the discriminator samples, in Hz, from 8000 to 1000000, and for Tier0 to 48000. 0 means 48000. The receiver runs at 48000, as sound cards sample, 24000, as MMDVM modems sample, 12000 or 9600: at Rate when it is one of them, else at the lowest of them at or above Rate, or 48000 above 48000, through a resampler. Tier0, also under TinyGo, resamples in fixed point with a table in flash. A lower rate costs less: the receiver halves 48 kHz and then 24 kHz with a half-band filter before its matched filter, which runs at 12 kHz or 9.6 kHz. |
| `Bits` | `int` | `16` | Bits is the resolution of the converter, from 8 to 16. 0 means 16. Each sample holds one code of Bits bits in its low bits. |
| `Unsigned` | `bool` | zero value | Unsigned selects offset binary codes, from 0 to 1<<Bits-1, in place of two's complement codes. |
| `OnBursts` | `func([]Burst)` | zero value | OnBursts, when not nil, selects stream mode: Write and Flush hand the bursts to it. Each call carries the bursts that are ready at the same point of the stream, in storage of the receiver that is valid only during the call: usually one, and after a late lock the bursts of the slots before the lock, decoded again. When nil, the receiver is in append mode, for AppendBursts and AppendFlush. |
| `PreEmphasis` | `*RxPreEmphasis` | zero value | PreEmphasis, when not nil, states that the radio de-emphasizes its discriminator output from 300 Hz to 3 kHz, as the speaker and accessory outputs of FM radios do, so that the receiver inverts it with the filter that it holds before it detects the bursts: a RxPreEmphasis of this receiver alone, set up by Init or NewRxPreEmphasis. Nil is a flat discriminator output, as the radio chips of hotspots and MMDVM modems give it. Every Tier and Rate takes it. A RxPreEmphasis that is not set up gives a *dmrgo.ConfigError. |
| `OnShortLC` | `func([]ShortLC)` | zero value | OnShortLC, when not nil, receives each Short LC that the CACH of the bursts carries and that passes its CRC-8, in stream and in append mode, in storage of the receiver that is valid only during the call: usually one. The receiver calls it as it outputs the burst of the last fragment, before that burst reaches OnBursts or the slice of AppendBursts. It decodes the fragments of the Decoded CACH as layer2.ShortLCAssembler does, and from Tier1 up decodes those that fail again from their soft values. Tier0 gives the hard decode only. Init and TinyGo builds take no OnShortLC and return a *dmrgo.ConfigError for it. |
<!-- /dmrgo:config -->

## How it works

DMR is 4FSK: each symbol is one of four frequency deviations and carries 2 bits. The receiver detects the symbols, searches for the SYNC patterns, locks to the slot timing and follows it, and decodes each FEC field with soft decisions. It also finds the superframe position of each voice burst, since bursts B to F carry no SYNC. (ETSI TS 102 361-1 clauses 6 and 10)

The tiers trade cost for sensitivity. Tier 0 slices the discriminator output and fits a microcontroller. Tier 1 adds the soft decoders. Tier 2 detects the bursts again coherently with a trellis that carries the carrier phase. Tier 3 is for offline decoding. The [Receiver datasheet](../rx/README.md) gives the sensitivity, compute and RAM of each tier.

The receiver gives bursts; the machines do the rest. The state machines: [voice Receiver](../machines/voice-receiver.md), [data Receiver](../machines/data-receiver.md), [bs Repeater](../machines/bs-repeater.md) and, for a trunked site, [trunk Follower](../machines/trunk-follower.md).

## Related guides

- [Follow calls on both slots of a bit stream](follow-both-slots.md)
- [Build and decode trunking messages](trunking.md)
- [Transcode voice: AMBE+2 FEC, Codec2, DVSI](transcode-voice.md)
- [Use an MMDVM modem](mmdvm-modem.md)
