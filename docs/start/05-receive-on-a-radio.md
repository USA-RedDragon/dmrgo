# Receive on a radio

This step recovers bursts from the I/Q samples of a software radio with an rf.Receiver, finds the time slot of each burst with an rf.Monitor, and follows the call with the voice.Receiver of step 3.

## From samples to bursts

A DMR signal is 4FSK, with two TDMA time slots on one carrier. A repeater sends a CACH between the bursts, which gives the time slot of the next burst. The receiver of [package rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf) does the 4FSK detection, the SYNC search and the slot timing, decodes each FEC field with soft decisions, and returns each burst as the same 33 bytes that steps 2 to 4 use. (ETSI TS 102 361-1 clauses 6 and 10)

## The capture

`v2/cmd/dmrrf/testdata/xpr8400_bs_voice_weak.cu8` is a weak RTL-SDR capture of the output of a Motorola XPR 8400 repeater, color code 7. It holds a group call from 3191868 to talkgroup 9 on time slot 2. The samples are 8-bit unsigned I/Q, the format that `rtl_sdr` writes, at the sample rate that the code gives. [v2/voice/testdata/xpr8400_bs_voice_weak.txt](../../v2/voice/testdata/xpr8400_bs_voice_weak.txt) lists the bursts of the same capture.

## The code

<!-- dmrgo:code file=v2/rf/example_test.go symbol=ExampleMonitor_capture -->
<!-- /dmrgo:code -->

Run it from the `v2` directory:

```sh
go test -run ExampleMonitor_capture -v ./rf
```

It prints:

<!-- dmrgo:output test=rf.ExampleMonitor_capture -->
<!-- /dmrgo:output -->

## What the code does

- [rf.NewReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewReceiver) makes the receiver from an [rf.RxConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#RxConfig): the sample rate, the sample format and the tier. The constructor allocates every buffer, and `Write` and `Flush` allocate nothing.
- `Tier` selects the detection and its compute cost. [Tiers](../concepts/tiers.md) compares the four tiers.
- With `OnBursts` set, the receiver is in stream mode. `rx.Write(iq)` takes samples in blocks of any size and calls `OnBursts` with the bursts that they complete. `rx.Flush()` ends the stream. With `OnBursts` nil, `rx.AppendBursts` and `rx.AppendFlush` append the bursts to a slice.
- The bursts in `OnBursts` are valid only during the call. Use them or copy them before the function returns.
- An [rf.Monitor](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Monitor) follows the TDMA channels of the stream, as a scanner does. `m.Burst(lb, &bs[i])` decodes the burst into the `layer2.Burst` `lb`, and returns its `layer2.Hint` with the time slot from the CACHs.
- `call.Decoded` takes the burst that the Monitor decoded, with that hint. The example keeps only the bursts of time slot 2. The time of each burst comes from `Burst.Seconds`, its time in the stream.

## Receive live

`Write` takes the samples as they come from the radio, so a live receiver is the same code with a reader in place of the file. The command `dmrrf rx` in `v2/cmd/dmrrf` reads I/Q or discriminator audio from a file or from standard input and prints each burst that it recovers, its decode, and the LC of each call. For example, with an RTL-SDR (put the frequency of your repeater in place of `<Hz>`):

```sh
rtl_sdr -f <Hz> -s 2048000 - | go run ./cmd/dmrrf rx -i - -format cu8 -rate 2048000
```

An FM radio with a discriminator output also works. [rf.NewAudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#NewAudioReceiver) takes the samples of the discriminator in place of I/Q.

The [Receiver datasheet](../rx/README.md) gives the sensitivity, the compute and the memory of each tier.

Next: [Next steps](06-next-steps.md).
