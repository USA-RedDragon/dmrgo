# Use basic privacy

This guide shows how to send and receive voice calls with Basic Privacy, the 16-bit key scheme of many radios, with package privacy.

## Goal

Apply Basic Privacy to the voice of a call that a [voice.Transmitter](../machines/voice-transmitter.md) sends, and remove it from the voice of a call that a [voice.Receiver](../machines/voice-receiver.md) gets, with the key number that the radios use.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- The packages [privacy](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy) and [voice](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice).
- The key number of the radios, 1 to 255 as a radio shows it, or the 16-bit key value.
- A radio service other than amateur. The amateur service forbids messages encoded to obscure their meaning (ITU Radio Regulations Article 25.2A, and for example 47 CFR 97.113(a)(4) in the United States), and the package refuses to encrypt in it.

Basic Privacy is a weak scheme: a fixed pattern from a 16-bit key XORs every voice frame, the same in every call. It hides a call from a casual listener only. Receiving encrypted traffic that you are not a party to can be unlawful, for example under 18 USC 2511 in the United States.

Run the example from the top of the repository:

```sh
go test -C v2 -run ExampleTransmitter -v ./privacy
```

## Steps

### 1. Make the key and the Transmitter

[privacy.NewBasicKeyNumber](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#NewBasicKeyNumber) returns the key of a key number as a radio shows it. [privacy.NewBasicKey](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#NewBasicKey) takes the 16-bit value instead. [privacy.NewTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#NewTransmitter) takes the key and the radio service of the station. It returns [ErrAmateur](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#ErrAmateur) for the amateur service, and an error for a service that is not stated.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=20-28 -->
<!-- /dmrgo:code -->

### 2. Make the Receiver

[privacy.NewReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#NewReceiver) makes a Receiver, and [Receiver.AddKey](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Receiver.AddKey) gives it the key. A Receiver holds one Basic Privacy key; a new one replaces it.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=29-33 -->
<!-- /dmrgo:code -->

### 3. Send the call

Set the Privacy service option and FID 0x10 in the [voice.Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Call), and call [Transmitter.Start](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Transmitter.Start) of the privacy Transmitter for each call. A Basic Privacy key ignores the message indicator, and sends no PI header. Then pass each output of the `voice.Transmitter` through [Transmitter.Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Transmitter.Output) before it goes on air. It changes the three AMBE+2 frames of each voice burst in place and leaves other bursts as they are.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=38-49 -->
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/privacy/example_test.go lines=57-67 -->
<!-- /dmrgo:code -->

### 4. Receive the call

Give each event of the `voice.Receiver` to [Receiver.Event](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Receiver.Event). For a voice event it writes the three frames of the burst, in the clear when it holds the key. It returns the [Info](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Info) of the call: `Encrypted` when the call carries privacy, `Decrypting` when the Receiver removes it.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=68-74 -->
<!-- /dmrgo:code -->

Some radios send Basic Privacy without the Privacy service option in the LC. [Receiver.ForceBasic](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Receiver.ForceBasic) applies the key to every voice call without a PI header.

## Output

The refusal in the amateur service, then the frames on air and after the Receiver:

<!-- dmrgo:output test=privacy.ExampleTransmitter -->
<!-- /dmrgo:output -->

The errors of the Transmitter:

<!-- dmrgo:errors pkg=privacy type=Transmitter -->
<!-- /dmrgo:errors -->

## How it works

ETSI TS 102 361 defines only the containers of privacy: the PI header burst and the Privacy bit of the service options. It does not define the schemes. A Basic Privacy key is a 48-bit pattern from the 16-bit key value, and the same pattern XORs every AMBE+2 frame. There is no PI header and no message indicator. A Receiver that joins a call late removes the privacy from the voice bursts after the LC of the call shows the Privacy option, or from every burst with ForceBasic. The value of a key number is the XOR of one base value for each bit set in the number; the base values come from public reverse engineering, and a public capture confirms number 100. (ETSI TS 102 361-1 clauses 5.1.2.2, 6.2 and 9.3.6, ETSI TS 102 361-2 clause 7.2.1)

The same package handles the ARC4, AES-128 and AES-256 schemes, which have a PI header with the algorithm, the key ID and a message indicator. For data, the Receiver reports Basic Privacy packets but leaves them as they are, and the Transmitter returns `ErrDataAlgorithm` for Basic Privacy data, as no public capture with a key proves their framing. Nothing else in dmrgo imports package privacy, so a program that does not import it sends and receives voice as before.

The bursts go to and from the voice machines: [voice Transmitter](../machines/voice-transmitter.md) and [voice Receiver](../machines/voice-receiver.md).

## Related guides

- [Rewrite and relay a voice call](relay-voice-call.md)
- [Transcode voice: AMBE+2 FEC, Codec2, DVSI](transcode-voice.md)
- [Transmit within a region's limits](transmit-region-limits.md)
- [Receive with an SDR or an FM radio](receive-sdr-fm.md)
