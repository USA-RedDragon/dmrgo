# Tiers

This page describes the four tiers of the receivers and transmitters of package rf: what each tier adds, what it costs, and how to pick one.

## What a tier is

A tier sets how a receiver of [package rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf) detects symbols, by compute budget. Each tier adds detection stages to the tier under it. A higher tier recovers bursts from a weaker signal and needs more CPU and more memory. Every tier decodes each FEC field with soft decisions, and every tier returns its bursts in the same form, through the same API: one config field changes the tier.

The tiers are the constants of [rf.Tier](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Tier). Their doc comments give what each tier adds:

<!-- dmrgo:code file=v2/rf/options.go symbol=TierDefault -->
```go
const (
	// TierDefault is the zero value: Tier1 for NewReceiver and
	// NewAudioReceiver, and Tier0 for Init and under TinyGo.
	TierDefault Tier = iota
	// Tier0 slices the output of an FM discriminator and its matched
	// filter. On I/Q from 48 kHz an FM discriminator at 12 kHz comes before
	// it, and the receiver keeps about 19 KB. On I/Q at 9.6 or 12 kHz that
	// a radio chip filtered to the channel it keeps about 1.4 KB, and Init
	// sets it up in memory of the caller, for a microcontroller. On I/Q
	// from 48 kHz Tier0 to Tier3 find a carrier up to 6.25 kHz off, half
	// the channel spacing, from the spectrum before they lock; a carrier
	// further off is taken for the next channel.
	Tier0
	// Tier1 adds to Tier0 the soft decoders: a BPTC(196,96) block whose
	// check fails by ordered statistics, C0 and C1 of each voice frame by
	// soft ML, the embedded LC of each superframe over its four fragments
	// and the earlier superframes of the call, and LookbackSlots slots
	// before the first lock decoded again. On I/Q it keeps the input
	// samples of those slots, and when it tuned to the carrier before the
	// lock it filters them again at the new tune before it decodes them. It
	// detects the bursts that they decode, and each rate 3/4 block, from
	// the symbols of the discriminator with the correlation of their noise.
	// It keeps about 35 KB on audio and 108 KB on I/Q, 58 KB of that for
	// the input samples.
	Tier1
	// Tier2 adds to the soft decoders of Tier1 the detection again of the
	// bursts with a trellis over three symbols that carries the carrier
	// phase with each survivor, from the SYNC or EMB of the burst, within a
	// compute budget per slot: on I/Q from the samples at 24 kHz, on audio
	// from the carrier phase that it sums from the discriminator samples.
	// It holds each burst one slot longer and keeps about 550 KB on audio
	// and 565 KB on I/Q, 75 KB of that for the input samples.
	Tier2
	// Tier3 on I/Q detects over a window of three symbols with one of 52
	// carrier phases in the trellis state, in two passes, detects each
	// voice burst again with the bits that its FEC fixed, and takes each
	// voice frame from it or from a Tier2 receiver beside it. It needs
	// about 70 times the compute of Tier2 and about 36 MB, for offline
	// decoding on a PC. On audio it is Tier2 with four passes over each
	// data burst, at about twice the compute of Tier2.
	Tier3
)
```
<!-- /dmrgo:code -->

The [Receiver datasheet](../rx/README.md) gives the measured sensitivity, CPU and RAM of each tier, for each input: FM discriminator audio, I/Q from a software radio, and I/Q that a radio chip filtered to the channel.

## Pick a tier

- **Tier0** is for a microcontroller. `Init` sets up a Tier0 receiver in memory that you give it, with no allocation. Under TinyGo, the zero tier is Tier0. Tier0 keeps no lookback, so it loses the start of a stream that it enters late.
- **Tier1** is the default of `NewReceiver` and `NewAudioReceiver` on a PC. It adds the soft decoders and decodes the slots before the first lock again.
- **Tier2** is for the best reception in real time on a PC. It adds coherent detection, holds each burst one slot longer, and needs more memory.
- **Tier3** is for offline decoding of a recording on a PC. It gives the best sensitivity on I/Q, and needs much more CPU and memory than Tier2.

Tier0 to Tier2 each run in real time on a PC. Step 5 of Get started, [Receive on a radio](../start/05-receive-on-a-radio.md), uses Tier1.

## Configure the tier

The `Tier` field of [rf.RxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#RxConfig) selects the tier of an I/Q receiver. The zero value `TierDefault` selects the default tier. The fields of the config:

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

[rf.AudioRxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioRxConfig) has the same `Tier` field for a receiver of discriminator audio. On audio, Tier3 is Tier2 with more passes over each data burst.

## Transmit tiers

The transmitter has tiers too. The `Tier` field of [rf.TxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#TxConfig) selects the modulator:

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
| `Power` | `int` | `top` | Power is the most power of the station, in dBm, from 1 to 63. 0 means the highest legal power of the Service in the Region. The limits are kept at the next of the breakpoints 27, 30, 40, 44, 47, 50, 54, 57, 60 and 62 dBm at or above it. |
<!-- /dmrgo:config -->

The [Transmitter datasheet](../tx/README.md) gives the limits, the formats, the rates and the measured margins of each transmit tier.

Next: [Get started](../start/01-install.md), if you have not done it yet.
