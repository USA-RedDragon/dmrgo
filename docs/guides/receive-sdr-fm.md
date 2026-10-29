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
<!-- /dmrgo:code -->

### 3. Decode discriminator audio

An FM radio gives the 4FSK signal at its discriminator output. [rf.NewAudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewAudioReceiver) makes an [AudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioReceiver) for the sample rate and the sample width of the audio. It has the same `Write`, `Flush` and `OnBursts` as the I/Q Receiver.

On a microcontroller, [AudioReceiver.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioReceiver.Init) sets up the receiver in Tier 0 in memory of the caller, without an allocation. The rxaudio example runs it as a hotspot board does, with a placeholder `read` in place of the ADC:

<!-- dmrgo:code file=v2/examples/rxaudio/main.go lines=9-35 -->
<!-- /dmrgo:code -->

It builds with TinyGo and `-gc=none`, which fails the link of any program that allocates. The page [Microcontrollers](../targets/mcu.md) gives the build.

## Output

The Monitor follows a group call on TDMA channel 2 of a repeater:

<!-- dmrgo:output test=rf.ExampleMonitor -->
<!-- /dmrgo:output -->

The rxaudio example on a PC, with silence as its input:

<!-- dmrgo:output example=rxaudio -->
<!-- /dmrgo:output -->

The configs of the two receivers:

<!-- dmrgo:config pkg=rf type=RxConfig -->
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=rf type=AudioRxConfig -->
<!-- /dmrgo:config -->

## How it works

DMR is 4FSK: each symbol is one of four frequency deviations and carries 2 bits. The receiver detects the symbols, searches for the SYNC patterns, locks to the slot timing and follows it, and decodes each FEC field with soft decisions. It also finds the superframe position of each voice burst, since bursts B to F carry no SYNC. (ETSI TS 102 361-1 clauses 6 and 10)

The tiers trade cost for sensitivity. Tier 0 slices the discriminator output and fits a microcontroller. Tier 1 adds the soft decoders. Tier 2 detects the bursts again coherently with a trellis that carries the carrier phase. Tier 3 is for offline decoding. The [Receiver datasheet](../rx.md) gives the sensitivity, compute and RAM of each tier.

The receiver gives bursts; the machines do the rest. The state machines: [voice Receiver](../machines/voice-receiver.md), [data Receiver](../machines/data-receiver.md), [bs Repeater](../machines/bs-repeater.md) and, for a trunked site, [trunk Follower](../machines/trunk-follower.md).

## Related guides

- [Transmit within a region's limits](transmit-region-limits.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md)
- [Build and decode trunking messages](trunking.md)
- [Transcode voice: AMBE+2 FEC, Codec2, DVSI](transcode-voice.md)
- [Use an MMDVM modem](mmdvm-modem.md)
