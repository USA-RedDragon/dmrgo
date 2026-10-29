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
```go
func ExampleRegionError() {
	_, err := rf.NewTransmitter(&rf.TxConfig{Region: rf.RegionUS, Service: rf.ServiceLicenseFree})
	var re *rf.RegionError
	if errors.As(err, &re) {
		fmt.Println(re.Field+":", re.Reason)
	}
}
```
<!-- /dmrgo:code -->

### 3. Transmit I/Q from a microcontroller

[Transmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Transmitter.Init) sets up a Tier 0 transmitter in memory of the caller, without an allocation. The txiq example feeds the I/Q DAC of a software radio in stream mode: `Write` takes bursts and hands the samples to `OnSamples`, `Active` reports whether the transmitter is keyed, and `Flush` ends the transmission. It stops when `Init` refuses the setup.

<!-- dmrgo:code file=v2/examples/txiq/main.go symbol=main -->
```go
func main() {
	err := tx.Init(rf.TxConfig{Rate: 96000, OnSamples: toDAC, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30})
	if err != nil {
		println("setup refused")
		return
	}
	x := byte(1)
	for i := range one[0].Data {
		x = x*37 + 11
		one[0].Data[i] = x
	}
	one[0].Stream = 1
	for i := range 100 {
		one[0].Slot = int64(2 * i)
		if _, err := tx.Write(one[:]); err != nil {
			return
		}
		keyed = tx.Active()
	}
	_ = tx.Flush()
	println("sum", sum, keyed)
}
```
<!-- /dmrgo:code -->

On a PC, [rf.NewTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewTransmitter) makes the same transmitter in the other tiers, at any rate of its config, and `AppendSamples` and `AppendFlush` append the samples to a slice when `OnSamples` is nil.

### 4. Transmit through an FM radio

An FM radio with a flat modulation input, such as a 9600 baud data port, takes the deviation signal as audio. [AudioTransmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioTransmitter.Init) sets it up for the rate and the resolution of the DAC. `Bits` and `Unsigned` give the codes of the converter. State a pre-emphasis or a low pass of the radio in `PreEmphasis` and `InputLowpassHz`, since both change the signal on air. The txaudio example keys the radio by `Active`:

<!-- dmrgo:code file=v2/examples/txaudio/main.go symbol=main -->
```go
func main() {
	err := tx.Init(rf.AudioTxConfig{Rate: 48000, Bits: 12, Unsigned: true, OnSamples: toDAC,
		Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30})
	if err != nil {
		println("setup refused")
		return
	}
	x := byte(1)
	for i := range one[0].Data {
		x = x*37 + 11
		one[0].Data[i] = x
	}
	one[0].Stream = 1
	for i := range 100 {
		one[0].Slot = int64(2 * i)
		if _, err := tx.Write(one[:]); err != nil {
			return
		}
		ptt = tx.Active()
	}
	_ = tx.Flush()
	println("sum", sum, ptt)
}
```
<!-- /dmrgo:code -->

### 5. Write a test transmission with dmrrf

`dmrrf tx` writes the bursts of a test sequence as I/Q or audio to a file or a pipe, for the transmit tool of your radio. The [README of dmrrf](../../v2/cmd/dmrrf/README.md) gives the sequences and the commands for a HackRF, a SoapySDR radio, GNU Radio and an FM transmitter.

## Output

A refused setup:

<!-- dmrgo:output test=rf.ExampleRegionError -->
```text
TxConfig.Service: the region has no license-free DMR service
```
<!-- /dmrgo:output -->

The txiq and txaudio examples on a PC. Each prints `setup refused` when `Init` refuses its setup:

<!-- dmrgo:output example=txiq -->
```text

```
<!-- /dmrgo:output -->

<!-- dmrgo:output example=txaudio -->
```text

```
<!-- /dmrgo:output -->

The configs, with the range and the default of each field:

<!-- dmrgo:config pkg=rf type=TxConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Tier` | `Tier` | `discDefaultTier` (2) or `Tier0` (1) | Tier selects the modulator: Tier0, Tier1 or Tier2. The zero value TierDefault is Tier1 for NewTransmitter, and Tier0 for Init and under TinyGo. Tier3 has no modulator of its own yet and gives a *dmrgo.ConfigError. |
| `Rate` | `int` | `defaultTxRate` (2400000) | Rate is the sample rate of the I/Q, in Hz. 0 means 2400000. Tier1 and Tier2 make any Rate from 48000 to 20000000, and Tier0 makes 96000. Under 62500 the image of the main lobe falls within two channels of the carrier, where no limit can count on a filter of the radio, so it gives a *RegionError. |
| `Format` | `IQFormat` | `IQFormatCS16` (3) | Format is the layout of the I/Q samples. 0 means IQFormatCS16. IQFormatCF32 values lie on the grid of IQFormatCS16. |
| `OffsetHz` | `float64` | zero value | OffsetHz is the frequency of the carrier relative to the center of the I/Q, in Hz, under Rate/4 in size. Tier0 rounds it to whole Hz. |
| `OnSamples` | `func([]byte)` | zero value | OnSamples, when not nil, selects stream mode: Write and Flush hand the samples to it in chunks of at most MaxOutput(0) samples, in storage of the transmitter that is valid only during the call. When nil, the transmitter is in append mode, for AppendSamples and AppendFlush. |
| `Region` | `Region` | zero value | Region is the regulatory region whose limits the signal keeps. |
| `Service` | `Service` | zero value | Service is the radio service of the station. ServiceUnknown keeps the limits of the land mobile and amateur services at once. A station of the license-free service must state it. |
| `Power` | `int` | `top` | Power is the most power of the station, in dBm, from 1 to 63. 0 means the highest legal power of the Service in the Region. |
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=rf type=AudioTxConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Tier` | `Tier` | `discDefaultTier` (2) or `Tier0` (1) | Tier selects the modulator: Tier0, Tier1 or Tier2. The zero value TierDefault is Tier1 for NewAudioTransmitter, and Tier0 for Init and under TinyGo. Tier3 has no modulator of its own yet and gives a *dmrgo.ConfigError. |
| `Rate` | `int` | `defaultTxAudio` (48000) | Rate is the sample rate of the converter, in Hz. 0 means 48000. Tier0 makes 24000 and 48000. Tier1 and Tier2 make any rate from 8000 whose ratio to 4800 has a numerator of at most 1024 in lowest terms, such as 8000, 22050 or 44100, and the limits are measured to 96000. |
| `Bits` | `int` | `16` | Bits is the resolution of the converter, from 8 to 16. 0 means 16. Each sample holds one code of Bits bits in its low bits. |
| `Unsigned` | `bool` | zero value | Unsigned selects offset binary codes, from 0 to 1<<Bits-1, in place of two's complement codes. |
| `OnSamples` | `func([]int16)` | zero value | OnSamples, when not nil, selects stream mode: Write and Flush hand the samples to it in chunks of at most MaxOutput(0) samples, in storage of the transmitter that is valid only during the call. When nil, the transmitter is in append mode, for AppendSamples and AppendFlush. |
| `Level` | `int` | not machine-readable; see the description | Level is the code of a run of symbol +3, 1944 Hz of deviation, less the center, from 1 to 16000. 0 means the most that fits in Bits with Offset over every run of symbols. The limits are measured at about half of full scale, 1040 codes at 12 bits. Under 16 bits, Tier1 and Tier2 at 24000 Hz and above reach 2.12 times Level, so there they are measured at the most that fits Bits at Offset 0, 965 to 981 codes at 12 bits. A Level under the measured level gives a *RegionError. |
| `Offset` | `int` | zero value | Offset is added to the center code, 0 or 1<<(Bits-1) when Unsigned. |
| `PreEmphasis` | `*PreEmphasis` | zero value | PreEmphasis, when not nil, states that the radio pre-emphasizes its modulator input from 300 Hz to 3 kHz, so that the transmitter inverts it with the modulator that it holds: a PreEmphasis of this transmitter alone, set up by Init or NewPreEmphasis. Nil is a radio with no pre-emphasis. Only Tier0 at Rate 48000 takes it, and only in the regions, services and powers where the limits keep their margin with it; elsewhere it gives a *RegionError. |
| `InputLowpassHz` | `int` | not machine-readable; see the description | InputLowpassHz states the cutoff in Hz of a low pass of at least two poles that the radio puts before its modulator input; 0 means none, a converter that drives the modulator with no filter. The limits are measured with a two-pole Butterworth low pass at 8000, 5000 and 3000 Hz, and a stated cutoff takes the next looser of these, so 6000 counts as 8000 and a cutoff above 8000 as none. A low pass lowers the least Rate that keeps the limits. A negative cutoff gives a *dmrgo.ConfigError. |
| `Region` | `Region` | zero value | Region is the regulatory region whose limits the signal keeps. |
| `Service` | `Service` | zero value | Service is the radio service of the station, as in TxConfig. |
| `Power` | `int` | `top` | Power is the most power of the station in dBm, as in TxConfig. |
<!-- /dmrgo:config -->

The errors of the transmitters:

<!-- dmrgo:errors pkg=rf type=Transmitter -->
| Error | Returned by | Meaning |
|---|---|---|
| [`dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) | `NewTransmitter`, `Transmitter.Init`, `Transmitter.Write` | ConfigError is the error of a constructor, and of a machine method that takes a config, option or argument field, for a field with a value out of range. |
| [`dmrgo.ErrNilReceiver`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilReceiver) | `Transmitter.Init`, `Transmitter.Write` | ErrNilReceiver is returned by a method of the library that returns an error when it is called on a nil pointer. |
| [`rf.ErrBurstOrder`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#ErrBurstOrder) | `Transmitter.Write` | ErrBurstOrder is the error of Write on a transmitter for a burst whose Slot does not increase within its Stream. |
| [`rf.ErrBurstService`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#ErrBurstService) | `Transmitter.Write` | ErrBurstService is the error of Write on a transmitter of the license-free service for a BS-sourced burst: the service allows no base station or repeater. |
| [`rf.ErrNotSetUp`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#ErrNotSetUp) | `Transmitter.Write` | ErrNotSetUp is the error of Write and Flush on a receiver or transmitter that neither its constructor nor Init set up. |
| [`rf.ErrTxTimeout`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#ErrTxTimeout) | `Transmitter.Write` | ErrTxTimeout is the error of Write on a transmitter of the license-free service for a burst that would carry one transmission past 179 s. |
| [`rf.RegionError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#RegionError) | `NewTransmitter`, `Transmitter.Init` | RegionError is the error of a transmitter setup that the Region, Service and Power do not allow, or that no measurement shows to keep their limits. |
<!-- /dmrgo:errors -->

<!-- dmrgo:errors pkg=rf type=AudioTransmitter -->
The doc comments of [`AudioTransmitter`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioTransmitter) and its methods name no error.
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
