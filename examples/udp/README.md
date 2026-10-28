# udp

This example sends filled forms as UDP datagrams over DMR confirmed data. It follows the 3270 pattern of emcomm data apps: both ends hold the form templates, and a unit sends only the data in the blanks. Two simulated radios share one simplex channel with bit errors. The example prints the size of each form encoding and the time from a form to its acknowledgement at rate 1/2, 3/4 and 1.

Run it from this directory:

```sh
go run . [-forms 30] [-seed 1]
```

The example is its own Go module, so its zstd dependency (github.com/klauspost/compress) does not become a dependency of the dmrgo library.

## The parts

- [link.go](link.go) is the UDP over DMR glue. `ip.AppendCompressed` puts a compressed UDP/IPv4 header (SAP UDP/IP header compression, ETSI TS 102 361-3 clause 7.2.3) on the datagram, and `data.Sender` sends it as a C_HEAD packet. On receive, `data.Receiver` gives the packet and `ip.Decode` gives the datagram back. The radio ID (LLID) carries the address, because a compressed header has no IPv4 addresses. `ip.RadioAddress` maps an LLID to an IPv4 address when a box needs one.
- [air.go](air.go) is the channel. Each bit of a burst flips with the given bit error rate (BER) at the radio that hears it. Two bursts that overlap in time are both lost. Each radio calls `Channel` on its `data.Sender` when it hears the channel busy, so the Senders use polite channel access.
- [app.go](app.go) is the form protocol. Each form gets a 16-bit form ID, and the far station sends an ACK datagram for each form it gets. The sender sends the form again when the ACK does not come. The receiver delivers each form ID one time. The application needs both:
  - A DMR ACK covers one radio hop, not the far application. A failover from another IP bearer to DMR also needs a record of what arrived.
  - The IP bearer service delivers a packet again after its response is lost (ETSI TS 102 361-3 clause 5.4.1.3).
- [form.go](form.go) and [codec.go](codec.go) are the template, the test forms and the encodings.

## Datagram format

Each datagram has a 5-octet form header and then the body:

| Octet | Content |
|---|---|
| 0 | kind (1 form, 2 ACK) in the high 4 bits, encoding in the low 4 bits |
| 1 to 2 | form ID, big endian |
| 3 | template ID |
| 4 | template version |
| 5 and on | body |

An ACK has only octets 0 to 2. The body of the `fields` encodings is one length octet and the value for each field of the template, in template order. The zstd encodings remove the 4-octet zstd magic number and the receiver puts it back.

## Results

These are the results of `go run .` with the default seed. The forms are service orders with 13 fields. The zstd dictionary is 4096 octets of other forms, made with another seed. The time is from the moment a form is ready to the moment its ACK arrives. A form gets 3 attempts by the application, and each attempt gets 8 DMR tries.

Octets of the form body, 200 forms, with a 4096-octet dictionary. Each datagram adds a 5-octet form header and a 9-octet compressed UDP/IP header:

| encoding | mean | min | max |
|---|---|---|---|
| screen | 380.0 | 348 | 398 |
| screen+zstd | 202.6 | 180 | 217 |
| fields | 116.6 | 81 | 139 |
| fields+zstd | 108.2 | 84 | 126 |
| fields+zstd+dict | 57.4 | 49 | 65 |

Time from a form to its acknowledgement, 30 forms each, one at a time:

| encoding | rate | BER | acked | mean s | p95 s | max s | bursts/form | DMR tries/form | app resends | copies dropped |
|---|---|---|---|---|---|---|---|---|---|---|
| screen | Rate 1/2 | 0 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.001 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.005 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.01 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.02 | 30/30 | 4.0 | 7.2 | 11.5 | 66.1 | 1.40 | 0 | 2 |
| screen | Rate 3/4 | 0 | 30/30 | 2.4 | 2.5 | 2.5 | 40.4 | 1.00 | 0 | 0 |
| screen | Rate 3/4 | 0.001 | 30/30 | 2.4 | 2.8 | 2.8 | 40.8 | 1.07 | 0 | 0 |
| screen | Rate 3/4 | 0.005 | 30/30 | 2.7 | 2.9 | 3.2 | 44.9 | 1.80 | 0 | 0 |
| screen | Rate 3/4 | 0.01 | 30/30 | 3.3 | 4.0 | 4.7 | 53.0 | 2.73 | 0 | 0 |
| screen | Rate 3/4 | 0.02 | 30/30 | 5.9 | 10.5 | 14.2 | 91.5 | 4.91 | 3 | 0 |
| screen | Rate 1 | 0 | 30/30 | 2.0 | 2.0 | 2.0 | 33.6 | 1.00 | 0 | 0 |
| screen | Rate 1 | 0.001 | 30/30 | 2.9 | 4.0 | 4.1 | 46.1 | 2.77 | 0 | 0 |
| screen | Rate 1 | 0.005 | 27/30 | 6.9 | 11.2 | 15.4 | 124.3 | 6.63 | 11 | 0 |
| screen | Rate 1 | 0.01 | 0/30 | - | - | - | 374.7 | 8.00 | 60 | 0 |
| screen | Rate 1 | 0.02 | 0/30 | - | - | - | 510.8 | 8.00 | 60 | 0 |
| fields+zstd+dict | Rate 1/2 | 0 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.001 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.005 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.01 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.02 | 30/30 | 1.5 | 2.1 | 2.2 | 24.6 | 1.03 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0 | 30/30 | 1.2 | 1.3 | 1.3 | 20.1 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.001 | 30/30 | 1.2 | 1.3 | 1.3 | 20.1 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.005 | 30/30 | 1.3 | 1.6 | 1.6 | 21.8 | 1.33 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.01 | 30/30 | 1.6 | 2.0 | 2.3 | 25.8 | 1.93 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.02 | 30/30 | 2.3 | 4.2 | 5.2 | 36.2 | 3.00 | 0 | 0 |
| fields+zstd+dict | Rate 1 | 0 | 30/30 | 1.1 | 1.1 | 1.1 | 19.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1 | 0.001 | 30/30 | 1.5 | 2.3 | 2.3 | 24.1 | 1.83 | 0 | 0 |
| fields+zstd+dict | Rate 1 | 0.005 | 30/30 | 3.6 | 10.0 | 14.7 | 51.0 | 4.50 | 4 | 1 |
| fields+zstd+dict | Rate 1 | 0.01 | 10/30 | 5.9 | 10.2 | 10.2 | 143.9 | 7.63 | 45 | 0 |
| fields+zstd+dict | Rate 1 | 0.02 | 0/30 | - | - | - | 187.7 | 8.00 | 60 | 0 |

The columns:

- bursts/form: all bursts on the channel in both directions, the ACK datagrams included.
- DMR tries/form: DMR transmissions of each form datagram. A try after the first one resends only the blocks that the last SACK requested.
- app resends: forms that the application sent again.
- copies dropped: forms that arrived again and that the receiver did not deliver again.

## What the results show

- Sending only the fields cuts a form from 380 to 117 octets. zstd without a dictionary gains little on a body this small (108 octets). zstd with a dictionary of earlier forms halves it again, to 57 octets.
- With a clean channel, a form takes 1.1 to 1.4 s with `fields+zstd+dict` and 2.0 to 3.4 s as a full screen. The 5 preamble CSBKs, the response wait and the ACK datagram take a large part of that time.
- Rate 1 has no FEC. It fails above a BER of about 0.005 to 0.01. Rate 1/2 stays flat up to a BER of 0.02. For a small form, rate 1/2 costs few bursts more than rate 1 (24 against 19 per form), so rate 1/2 is a good default for a mobile unit.
- A zstd frame with a dictionary carries the dictionary ID. A receiver with another dictionary returns an error for the frame and does not decode it into wrong data (`TestDictionaryMismatch`). Give each new dictionary a new ID, and change the template version with it.
- The compressed UDP/IP header is 9 octets because port 3270 needs an extended header for each port. Ports 5016 and 5017 need none (5 octets), but radios read those ports as text messages and location data.

## Limits of the simulation

- The bit errors are independent. A real mobile channel has fades, which lose bursts in groups.
- The two radios talk direct on one simplex channel, with no repeater and no network leg.
- The Senders use the default of 5 preamble CSBKs before each packet that does not follow a recent response.
- The ACK is its own confirmed DMR packet.
