# Report and request location: LIP, LRRP, NMEA

This guide shows how to request the position of a radio and read the position that it reports, in the LRRP, NMEA and LIP formats that DMR radios use.

## Goal

Send a location request to a radio, build its answer, and read the position from the answer.

## What you need

- Go and a checkout of dmrgo. The code on this page comes from Go Example functions, which `go test` runs.
- The packages [location](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location), [mototrbo/lrrp](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp), [nmea](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea), [lip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip) and [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip).
- A way to send data packets. [Send SMS and confirmed data with retries](send-data.md) shows how.

Run the examples from the top of the repository:

```sh
go test -C v2 -run Example -v ./location ./lip
```

Which format to use depends on the radio:

| Radios | Format | Carried in |
| --- | --- | --- |
| MOTOTRBO, and Hytera | LRRP request and report | UDP datagram to port <!-- dmrgo:const pkg=ip name=PortLRRP -->`4001`<!-- /dmrgo:const --> |
| AnyTone | NMEA RMC sentence | UDP datagram to port <!-- dmrgo:const pkg=ip name=PortNMEA -->`10110`<!-- /dmrgo:const --> |
| Hytera | HDAP location protocol | UDP datagram to port <!-- dmrgo:const pkg=ip name=PortHyteraLocation -->`3003`<!-- /dmrgo:const --> |
| Radios and systems that use ETSI LIP | LIP PDU | appended data of a UDT (ETSI TS 102 361-4) |

## Steps

### 1. Request a position with LRRP

Make an [lrrp.Message](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp#Message) with an immediate request document and a request ID, and write it with `AppendBinary`. Put it in a UDP datagram to the LRRP port with [ip.AppendIPv4](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#AppendIPv4). [ip.RadioAddress](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#RadioAddress) gives the IPv4 address of each radio ID. Send the datagram in a data packet with the SAP IP based packet data.

<!-- dmrgo:code file=v2/location/example_test.go symbol=requestLRRP -->
```go
func requestLRRP(server, radio uint32) (ip.Datagram, []byte, error) {
	request = lrrp.Message{Document: lrrp.DocumentImmediateRequest}
	request.Request.RequestID, request.Request.HasRequestID = []byte{0x24, 0x68, 0xac, 0xe0}, true
	request.Request.AccuracyAndTime = true
	payload, err := request.AppendBinary(nil)
	if err != nil {
		return ip.Datagram{}, nil, err
	}
	d := ip.Datagram{
		SourcePort: ip.PortLRRP, DestinationPort: ip.PortLRRP,
		Source: ip.RadioAddress(ip.DefaultNetwork, server), Destination: ip.RadioAddress(ip.DefaultNetwork, radio), Payload: payload,
	}
	data, err := ip.AppendIPv4(nil, &d)
	if err != nil {
		return ip.Datagram{}, nil, err
	}
	var h pdu.DataHeader
	if err := h.SetConfirmed(pdu.ConfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: radio, LLIDSource: server,
	}); err != nil {
		return ip.Datagram{}, nil, err
	}
	_, err = decoder.Decode(&h, data)
	fmt.Println("request:", err)
	return d, data, nil
}
```
<!-- /dmrgo:code -->

[location.Decoder.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#Decoder.Decode) returns `location.ErrNoFix` for a request, so a program that reads every data packet on a channel can give each one to the Decoder.

### 2. Answer the request and read the position

The radio reads the request with [lrrp.DecodeMessageInto](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp#DecodeMessageInto) and answers with an immediate report that carries the same request ID, a result code and a position. The Decoder reads the answer into a [location.Fix](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#Fix), with the request ID that it answers.

<!-- dmrgo:code file=v2/location/example_test.go symbol=answerLRRP -->
```go
func answerLRRP(server, radio uint32, d ip.Datagram, data []byte) error {
	got, err := ip.Decode(enums.SAPIPBasedPacketData, data)
	if err != nil {
		return err
	}
	if err := lrrp.DecodeMessageInto(&request, got.Payload); err != nil {
		return err
	}
	report = lrrp.Message{Document: lrrp.DocumentImmediateReport}
	r := &report.Report
	r.RequestID, r.HasRequestID = request.Request.RequestID, true
	r.Result, r.HasResult = lrrp.ResultSuccess, true
	r.Circle2D, r.HasCircle2D = lrrp.Circle2D{Latitude: 34.212166, Longitude: 108.833333, Radius: 5}, true
	payload, err := report.AppendBinary(nil)
	if err != nil {
		return err
	}
	d.Source, d.Destination, d.Payload = d.Destination, d.Source, payload
	data, err = ip.AppendIPv4(nil, &d)
	if err != nil {
		return err
	}
	var h pdu.DataHeader
	if err := h.SetUnconfirmed(pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: server, LLIDSource: radio,
	}); err != nil {
		return err
	}
	f, err := decoder.Decode(&h, data)
	if err != nil {
		return err
	}
	fmt.Printf("%s from %d: %.5f %.5f, %.0f m, request %x, result %d\n",
		f.Format, f.Source, f.Latitude, f.Longitude, f.Radius, f.RequestID[:f.RequestIDLength], f.Result)
	return nil
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=location.ExampleDecoder_Decode_lrrp -->
```text
request: location: packet carries no position
LRRP from 3191868: 34.21217 108.83333, 5 m, request 2468ace0, result 0
```
<!-- /dmrgo:output -->

An `lrrp.Message` is large, and a `location.Decoder` holds one. Keep them in a struct or a package variable, not in a local variable on a small stack, as the examples do.

### 3. Send a position as NMEA

[location.AppendPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#AppendPacket) writes a Fix as the NMEA RMC sentence of an AnyTone radio, behind a compressed UDP/IPv4 header in an unconfirmed packet, and sets the data header in a [location.Header](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#Header). The same Decoder reads it back.

<!-- dmrgo:code file=v2/location/example_test.go symbol=ExampleAppendPacket -->
```go
func ExampleAppendPacket() {
	f := location.Fix{
		Format: location.FormatNMEA, Source: 3191868, Destination: 3140598, Valid: true,
		Latitude: 34.212166, Longitude: 108.833333, Course: 121, HasCourse: true, HasSpeed: true,
		Year: 2025, Month: 12, Day: 2, Hour: 20, Minute: 0, Second: 37, HasTime: true,
	}
	data, err := location.AppendPacket(nil, &f, &header)
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header.Data, data, enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	got, err := decoder.Decode(&p.Header, data)
	fmt.Printf("%d blocks: %q\n%s from %d: %.5f %.5f %v\n", len(p.Blocks), data[9:], got.Format, got.Source, got.Latitude, got.Longitude, err)
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=location.ExampleAppendPacket -->
```text
8 blocks: "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n"
NMEA from 3191868: 34.21217 108.83333 <nil>
```
<!-- /dmrgo:output -->

To read or write the sentence alone, use [nmea.ParseRMC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea#ParseRMC) and [nmea.AppendRMC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea#AppendRMC). `ParseRMC` returns an error for a sentence with a bad checksum.

### 4. Request a position with LIP

A LIP PDU is a string of bits. Encode an [lip.ImmediateRequest](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#ImmediateRequest) with the report type that you want, and put it in the appended data of a UDT with [trunk.Appended.SetLIP](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Appended.SetLIP). The radio reads the PDU type with [lip.DecodeHeader](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#DecodeHeader) and the request with `DecodeImmediateRequest`. (ETSI TS 102 361-4 clause B.3.10, ETSI TS 100 392-18-1 clause 6.2.16)

<!-- dmrgo:code file=v2/lip/example_test.go symbol=requestLIP -->
```go
func requestLIP(a *trunk.Appended) ([]byte, error) {
	q := lip.ImmediateRequest{ReportType: lip.ReportLongPosition}
	req, _, err := q.Encode(nil)
	if err != nil {
		return nil, err
	}
	if err := a.SetLIP(req); err != nil {
		return nil, err
	}
	fmt.Println(a.Format, a.Blocks, "block")
	got, err := a.LIP(nil)
	if err != nil {
		return nil, err
	}
	h, err := lip.DecodeHeader(got)
	if err != nil {
		return nil, err
	}
	if h.Extension != lip.ExtensionImmediateReport {
		return nil, fmt.Errorf("lip: got %v", h.Extension)
	}
	in, err := lip.DecodeImmediateRequest(got)
	if err != nil {
		return nil, err
	}
	fmt.Println("request:", in.ReportType)
	return got, nil
}
```
<!-- /dmrgo:code -->

### 5. Answer with a LIP long location report

The radio answers with an [lip.LongReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#LongReport) of the type that the request specified, here with the time of the position. [lip.DecodeLongReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#DecodeLongReport) reads it. Velocities and accuracies stay codes, as the spec codes them in steps. (ETSI TS 100 392-18-1 clause 6.2.2)

<!-- dmrgo:code file=v2/lip/example_test.go symbol=answerLIP -->
```go
func answerLIP(a *trunk.Appended, buf []byte) error {
	r := lip.LongReport{
		Time:     lip.Time{Type: lip.TimePosition, Day: 2, Hour: 20, Minute: 0, Second: 37},
		Location: lip.Location{Shape: lip.ShapePoint, Latitude: 34.212166, Longitude: 108.833333},
	}
	answer, _, err := r.Encode(nil)
	if err != nil {
		return err
	}
	if err := a.SetLIP(answer); err != nil {
		return err
	}
	got, err := a.LIP(buf[:0])
	if err != nil {
		return err
	}
	rep, err := lip.DecodeLongReport(got)
	if err != nil {
		return err
	}
	t := rep.Time
	fmt.Printf("answer: %v %.4f %.4f, day %d %02d:%02d:%02d UTC\n", rep.Location.Shape, rep.Location.Latitude, rep.Location.Longitude, t.Day, t.Hour, t.Minute, t.Second)
	return nil
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=lip.ExampleDecodeImmediateRequest -->
```text
LIP 1 block
request: Long location report with time of position
answer: Location point 34.2122 108.8333, day 2 20:00:37 UTC
```
<!-- /dmrgo:output -->

## How it works

The LRRP, NMEA and HDAP formats all travel in UDP datagrams in DMR data packets. `location.Decoder` reads the datagram with [ip.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Decode), for a full IPv4 header or a compressed one, and selects the format from the destination UDP port. It returns one `Fix` for all formats, with `Format` set to the format of the report. For an answer without a position, such as the answer of a radio without a GNSS fix, it returns the Fix with its result code and `location.ErrNoPosition`.

An LRRP message is a document of tokens in MOTOTRBO binary XML. A triggered request requests reports on a period or a distance, and a stop request ends it. The [lrrp](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp) package doc describes the coding of the latitude and longitude.

LIP is the location protocol of ETSI TS 100 392-18-1. A DMR UDT carries it in the LIP format, and the USBD polling service of a trunked system compresses it. The lip package reads and writes the long location report, the report acknowledgement, the immediate request and the enable and disable request and response. [trunk.DecodeLIPShortReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#DecodeLIPShortReport) reads the short location report. (ETSI TS 102 361-4 clause B.3.10, ETSI TS 100 392-18-1 clause 6)

To send the data packets on a channel, use the [data Sender](../machines/data-sender.md) and read them with the [data Receiver](../machines/data-receiver.md). On a Tier III system, a UDT goes through the short data services of the [trunk MS](../machines/trunk-ms.md).

## Related guides

- [Send SMS and confirmed data with retries](send-data.md)
- [Run TCP or UDP over DMR data](tcp-udp-over-dmr.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md), which reads the GPS position in the embedded LC of a voice call
