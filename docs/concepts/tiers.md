# Tiers

This page describes the four tiers of the receivers and transmitters of package rf: what each tier adds, what it costs, and how to pick one.

## What a tier is

A tier sets how a receiver of [package rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf) detects symbols, by compute budget. Each tier adds detection stages to the tier under it. A higher tier recovers bursts from a weaker signal and needs more CPU and more memory. Every tier decodes each FEC field with soft decisions, and every tier returns its bursts in the same form, through the same API: one config field changes the tier.

The tiers are the constants of [rf.Tier](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Tier). Their doc comments give what each tier adds:

<!-- dmrgo:code file=v2/rf/receiver.go symbol=TierDefault -->
<!-- /dmrgo:code -->

The [Receiver datasheet](../rx.md) gives the measured sensitivity, CPU and RAM of each tier, for each input: FM discriminator audio, I/Q from a software radio, and I/Q that a radio chip filtered to the channel.

## Pick a tier

- **Tier0** is for a microcontroller. `Init` sets up a Tier0 receiver in memory that you give it, with no allocation. Under TinyGo, the zero tier is Tier0. Tier0 keeps no lookback, so it loses the start of a stream that it enters late.
- **Tier1** is the default of `NewReceiver` and `NewAudioReceiver` on a PC. It adds the soft decoders and decodes the slots before the first lock again.
- **Tier2** is for the best reception in real time on a PC. It adds coherent detection, holds each burst one slot longer, and needs more memory.
- **Tier3** is for offline decoding of a recording on a PC. It gives the best sensitivity on I/Q, and needs much more CPU and memory than Tier2.

Tier0 to Tier2 each run in real time on a PC. Step 5 of Get started, [Receive on a radio](../start/05-receive-on-a-radio.md), uses Tier1.

## Configure the tier

The `Tier` field of [rf.RxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#RxConfig) selects the tier of an I/Q receiver. The zero value `TierDefault` selects the default tier. The fields of the config:

<!-- dmrgo:config pkg=rf type=RxConfig -->
<!-- /dmrgo:config -->

[rf.AudioRxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioRxConfig) has the same `Tier` field for a receiver of discriminator audio. On audio, Tier3 is Tier2 with more passes over each data burst.

## Transmit tiers

The transmitter has tiers too. The `Tier` field of [rf.TxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#TxConfig) selects the modulator:

<!-- dmrgo:config pkg=rf type=TxConfig -->
<!-- /dmrgo:config -->

The [Transmitter datasheet](../tx.md) gives the limits, the formats, the rates and the measured margins of each transmit tier.

Next: [Get started](../start/01-install.md), if you have not done it yet.
