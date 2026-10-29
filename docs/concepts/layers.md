# The layer model

This page shows how the packages of dmrgo stack, from the bits of a burst to the applications in data packets, and where the networks and the radio connect.

## The stack

The packages follow the layers of ETSI TS 102 361. Each layer uses only the layers under it. A program uses the layers that its task needs.

| Layer | What it holds | Packages |
| --- | --- | --- |
| Applications | The contents of data packets: IP, text messages, positions and the MOTOTRBO and Hytera services. | [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip), [sms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms), [shortdata](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/shortdata), [location](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location), [lip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip), [nmea](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea), [mototrbo/ars](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars), [mototrbo/lrrp](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp), [mototrbo/tms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms), [hytera/hdap](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/hytera/hdap) |
| Services | The calls and the control of one logical channel or of a site, as state machines without I/O. | [voice](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice), [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data), [csbk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk), [bs](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs), [trunk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk), [trunk/trunkauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk/trunkauth), [privacy](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy) |
| Layer 3 | The information elements that PDUs carry, such as service options and channel definitions. | [layer3](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer3) |
| Layer 2 | Bursts, PDUs, FEC, the CACH, voice superframes and data packets. | [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2), [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu), [layer2/packet](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet), [layer2/framer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/framer), [fec](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec), [enums](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/enums), [constants](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/constants) |
| Radio | I/Q samples and FM audio to bursts and back, and the voice codecs. | [rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf), [vocoder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder), [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2), [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi) |

The networks are beside the radio. A network carries bursts between hosts, in place of the air:

| Network | Packages |
| --- | --- |
| The MMDVM network protocol, the HomeBrew Repeater Protocol (HBRP), the MMDVMHost to DMRGateway link and OpenBridge | [transport/dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd), [transport/dmrd/dmrdauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth), [transport/dmrd/dmrdsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession) |
| Motorola IP Site Connect (IPSC) | [transport/ipsc](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc), [transport/ipsc/ipscauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth), [transport/ipsc/ipscsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession) |
| Hytera IP Multi-Site Connect | [transport/hytera](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera) |
| The serial protocol of an MMDVM modem | [transport/mmdvm](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm) |

The [package documentation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) gives one line for each package.

## The burst connects the layers

Every path through the stack passes a 33-byte burst, a [layer2.RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurst), with a [layer2.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) beside it. The hint holds the data of the source about the burst besides its bits: the slot, the burst kind, the voice position, the data type, a sequence number, a stream ID, and the end and loss flags of a transport.

- The radio side gives bursts: [rf.Burst.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Burst.Hint), or the [rf.Monitor](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Monitor) for the time slot from the CACH.
- Each network gives bursts with a hint: [dmrd.Data.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data.Hint), [ipsc.User.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.Hint), [hytera.Frame.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#Frame.Hint) and [mmdvm.DMRData.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DMRData.Hint).
- The services take a burst and a hint in `Burst` or `Decoded`, and return bursts to send, each with its hint.

So one service machine works with every source. A `voice.Receiver` follows a call from an RTL-SDR, from an MMDVM modem or from a network master with the same code. The [Get started](../start/01-install.md) steps show this: steps 3 and 5 give the same machine bursts from a file and from I/Q.

## Where the layers come from

- Layer 2 and the radio follow ETSI TS 102 361-1, the air interface. (ETSI TS 102 361-1 clauses 6, 9 and 10)
- The voice and generic services, the CSBK services and the Full LC follow ETSI TS 102 361-2. (ETSI TS 102 361-2 clause 5)
- Packet data and its applications follow ETSI TS 102 361-3. (ETSI TS 102 361-3 clauses 5 and 6)
- Trunking follows ETSI TS 102 361-4. (ETSI TS 102 361-4 clauses 6.2 to 6.6)

The networks, the MOTOTRBO and Hytera services, and the vocoders are not in ETSI TS 102 361. Each package doc gives the source that it follows. [ETSI coverage](../etsi.md) gives the status of each clause.

Next: [I/O-free services](services.md).
