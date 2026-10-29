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
```go
key := privacy.NewBasicKeyNumber(100)
if _, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: key, Service: privacy.ServiceAmateur}); errors.Is(err, privacy.ErrAmateur) {
	fmt.Println("amateur:", err)
}
ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: key, Service: privacy.ServiceLandMobile})
if err != nil {
	fmt.Println(err)
	return
}
```
<!-- /dmrgo:code -->

### 2. Make the Receiver

[privacy.NewReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#NewReceiver) makes a Receiver, and [Receiver.AddKey](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Receiver.AddKey) gives it the key. A Receiver holds one Basic Privacy key; a new one replaces it.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=29-33 -->
```go
prx := privacy.NewReceiver()
if err := prx.AddKey(key); err != nil {
	fmt.Println(err)
	return
}
```
<!-- /dmrgo:code -->

### 3. Send the call

Set the Privacy service option and FID 0x10 in the [voice.Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Call), and call [Transmitter.Start](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Transmitter.Start) of the privacy Transmitter for each call. A Basic Privacy key ignores the message indicator, and sends no PI header. Then pass each output of the `voice.Transmitter` through [Transmitter.Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Transmitter.Output) before it goes on air. It changes the three AMBE+2 frames of each voice burst in place and leaves other bursts as they are.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=38-49 -->
```go
call := voice.Call{Group: true, Target: 9, Source: 3191868, Options: layer3.ServiceOptions{IsPrivacy: true}, FID: enums.FIDMotorola}
ptx.Start(0)
if err := tx.Start(0, call, nil); err != nil {
	fmt.Println(err)
	return
}
sent := make([]vocoder.AMBEFrame, 18)
for i := range sent {
	sent[i] = vocoder.AMBEFrame{byte(i), 0x5A, 0xA5, byte(3 * i), 0x0F, 0xF0, 0x80}
	_ = tx.Frame(sent[i])
}
tx.Stop(0)
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/privacy/example_test.go lines=57-67 -->
```go
if o.Kind != voice.OutputBurst {
	continue
}
if err := ptx.Output(&o); err != nil {
	fmt.Println(err)
	return
}
if o.Hint.Kind == layer2.KindVoice {
	frames, _ := o.Burst.AMBEFrames()
	onAir = append(onAir, frames[:]...)
}
```
<!-- /dmrgo:code -->

### 4. Receive the call

Give each event of the `voice.Receiver` to [Receiver.Event](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Receiver.Event). For a voice event it writes the three frames of the burst, in the clear when it holds the key. It returns the [Info](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Info) of the call: `Encrypted` when the call carries privacy, `Decrypting` when the Receiver removes it.

<!-- dmrgo:code file=v2/privacy/example_test.go lines=68-74 -->
```go
for _, e := range rx.Burst(o.Due, &o.Burst, o.Hint, events[:0]) {
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	info = prx.Event(&e, &frames)
	if e.Kind == voice.EventVoice {
		received = append(received, frames[:]...)
	}
}
```
<!-- /dmrgo:code -->

Some radios send Basic Privacy without the Privacy service option in the LC. [Receiver.ForceBasic](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#Receiver.ForceBasic) applies the key to every voice call without a PI header.

## Output

The refusal in the amateur service, then the frames on air and after the Receiver:

<!-- dmrgo:output test=privacy.ExampleTransmitter -->
```text
amateur: privacy: the amateur service forbids encryption
on air: 0 of 18 frames in the clear
received: 18 of 18 frames in the clear
encrypted true PI header false decrypting true
```
<!-- /dmrgo:output -->

The errors of the Transmitter:

<!-- dmrgo:errors pkg=privacy type=Transmitter -->
| Error | Returned by | Meaning |
|---|---|---|
| [`dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) | `Transmitter.Data` | ConfigError is the error of a constructor, and of a machine method that takes a config, option or argument field, for a field with a value out of range. |
| [`dmrgo.ErrNilArgument`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilArgument) | `Transmitter.Output` | ErrNilArgument is returned by a function or method of the library that returns an error when a pointer, func or interface argument it needs is nil. |
| [`privacy.ErrDataAlgorithm`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#ErrDataAlgorithm) | `Transmitter.Data` | ErrDataAlgorithm is returned by Transmitter.Data for a key other than ARC4: no public capture with a key proves the data framing of AES, DES or Basic Privacy. |
| [`privacy.ErrNotStarted`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy#ErrNotStarted) | `Transmitter.Output` | ErrNotStarted is returned by Transmitter.Output before Start. |
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
