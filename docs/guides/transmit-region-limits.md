# Transmit within a region's limits

This guide shows how to turn DMR bursts into I/Q samples for a software radio or into modulator audio for an FM radio, with a setup that states the region, the radio service and the power of the station.

## Goal

Make the transmit signal of a station with package rf, so that the signal is never the cause of a failure of the limits of its region, and handle a setup that the transmitter refuses.

## What you need

- Go and a checkout of dmrgo. The examples run offline and write no signal to a radio.
- The package [rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf).
- The [Region](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Region) where the station operates, its radio [Service](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Service), and its highest power in dBm.
- To go on air, a software radio with a transmit tool, or an FM radio with a flat modulation input, a license for the frequency, and a dummy load or an attenuator for the first tests.

dmrgo makes no claim that a station complies with its rules. A setup that the transmitter accepts does not make the signal the cause of a failure of the limits that the library measured. The station, its radio and its operation stay the responsibility of the licensee.

## Steps

### 1. State the region, the service and the power

[TxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#TxConfig) for I/Q and [AudioTxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioTxConfig) for audio have the same three fields: `Region`, `Service` and `Power`. The zero values are the strictest setup: `RegionWorld` keeps the limits of every region at once, `ServiceUnknown` keeps those of the land mobile and amateur services at once, and a `Power` of 0 means the highest legal power of the service in the region. State each field to get the setup of your station. A station of the license-free service must state it.

### 2. Make the transmitter and check the error

[rf.NewTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewTransmitter) and [rf.NewAudioTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewAudioTransmitter) return a [*RegionError](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#RegionError) for a setup that the region, the service and the power do not allow, or that no measurement shows to keep their limits with margin. `Field` names the config field, and `Reason` gives the cause. A field out of range gives a `*dmrgo.ConfigError` instead. Do not transmit when either comes back.

<!-- dmrgo:code file=v2/rf/region_example_test.go symbol=ExampleRegionError -->
<!-- /dmrgo:code -->

### 3. Transmit I/Q from a microcontroller

[Transmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Transmitter.Init) sets up a Tier 0 transmitter in memory of the caller, without an allocation. The txiq example feeds the I/Q DAC of a software radio in stream mode: `Write` takes bursts and hands the samples to `OnSamples`, `Active` reports whether the transmitter is keyed, and `Flush` ends the transmission. It stops when `Init` refuses the setup.

<!-- dmrgo:code file=v2/examples/txiq/main.go symbol=main -->
<!-- /dmrgo:code -->

On a PC, [rf.NewTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewTransmitter) makes the same transmitter in the other tiers, at any rate of its config, and `AppendSamples` and `AppendFlush` append the samples to a slice when `OnSamples` is nil.

### 4. Transmit through an FM radio

An FM radio with a flat modulation input, such as a 9600 baud data port, takes the deviation signal as audio. [AudioTransmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioTransmitter.Init) sets it up for the rate and the resolution of the DAC. `Bits` and `Unsigned` give the codes of the converter. State a pre-emphasis or a low pass of the radio in `PreEmphasis` and `InputLowpassHz`, since both change the signal on air. The txaudio example keys the radio by `Active`:

<!-- dmrgo:code file=v2/examples/txaudio/main.go symbol=main -->
<!-- /dmrgo:code -->

### 5. Write a test transmission with dmrrf

`dmrrf tx` writes the bursts of a test sequence as I/Q or audio to a file or a pipe, for the transmit tool of your radio. The [README of dmrrf](../../v2/cmd/dmrrf/README.md) gives the sequences and the commands for a HackRF, a SoapySDR radio, GNU Radio and an FM transmitter.

## Output

A refused setup:

<!-- dmrgo:output test=rf.ExampleRegionError -->
<!-- /dmrgo:output -->

The txiq and txaudio examples on a PC. Each prints `setup refused` when `Init` refuses its setup:

<!-- dmrgo:output example=txiq -->
<!-- /dmrgo:output -->

<!-- dmrgo:output example=txaudio -->
<!-- /dmrgo:output -->

The configs, with the range and the default of each field:

<!-- dmrgo:config pkg=rf type=TxConfig -->
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=rf type=AudioTxConfig -->
<!-- /dmrgo:config -->

The errors of the transmitters:

<!-- dmrgo:errors pkg=rf type=Transmitter -->
<!-- /dmrgo:errors -->

<!-- dmrgo:errors pkg=rf type=AudioTransmitter -->
<!-- /dmrgo:errors -->

## How it works

The transmitter makes DMR 4FSK on a carrier, with the power ramps of TDMA bursts in the guard time of MS bursts. A BS-sourced burst keeps the carrier up into the next slot. It places each burst on the 30 ms slot grid by its stream and slot, and a new stream keys down and up again. (ETSI TS 102 361-1 clause 10)

Each region and service has an emission mask and spurious domain limits. Tests of the library measure the signal of each tier, rate and setup against them, and the results are tables in the code. A setup is accepted only where the measured signal keeps the limits with margin. The [Transmitter datasheet](../tx/README.md) gives the limits, the margins and the cost of each tier, generated from those tables. Because the tables change as measurements are added, this guide does not repeat them.

The bursts come from the service machines: [voice Transmitter](../machines/voice-transmitter.md), [data Sender](../machines/data-sender.md), [csbk Requester](../machines/csbk-requester.md) and [bs Repeater](../machines/bs-repeater.md).

## Related guides

- [Receive with an SDR or an FM radio](receive-sdr-fm.md)
- [Use basic privacy](basic-privacy.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Send SMS and confirmed data with retries](send-data.md)
