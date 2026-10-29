# Go API

This page links the documentation of each public package of the `v2` module on pkg.go.dev, in the groups of the package overview.

The root package [dmrgo](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) holds the overview of the module, the errors that every package shares and the types that several packages use. Its package doc lists the packages:

<!-- dmrgo:doc pkg=. section=Packages -->
Air interface, layers 2 and 3:

  - enums: the enumerated information elements, such as data types, SYNC patterns, FLCO, FID and color codes.
  - constants: the timers and constants of the Annexes of ETSI TS 102 361.
  - fec: the FEC result that decoders return, and ErrUncorrectable.
  - layer2: bursts, the byte-level RawBurst, the embedded LC, CACH and Short LC, the Reverse Channel, and the voice superframe transmitter and receiver.
  - layer2/pdu: the layer 2 PDUs (CSBK, Full LC, Short LC, data headers, MBC, PI header, slot type, EMB and data blocks) and their byte-level editors.
  - layer2/packet: segmentation and reassembly of data packets, with confirmed delivery and response packets.
  - layer2/framer: finds bursts in a demodulated bit stream.
  - layer3: the layer 3 information elements that PDUs carry, such as service options and channel definitions.

Services of a channel and of a site:

  - voice: the voice calls of one logical channel: a Receiver that turns bursts into call events, and a Transmitter.
  - data: data calls on one logical channel: a Receiver that delivers packets and answers confirmed ones, and a Sender with channel access and retries.
  - csbk: the CSBK services of one logical channel, without I/O: a request with its answer and retries, the answers of a radio, Pre\_CSBKs and the wakeup of a BS.
  - bs: the base station of a two-slot carrier: a repeater, a hotspot or the slot arbiter of a network server.
  - trunk: the Tier III channel tables, a Follower that follows the calls of a trunked site, an MS that registers and makes calls, and a TSCC that runs a single site.
  - trunk/trunkauth: the answer of an MS to the authentication challenge of a TSCC.

Applications in data packets:

  - ip: the IPv4 and UDP headers of data packets, and the compressed UDP/IPv4 header.
  - sms: text messages in every format that radios use.
  - shortdata: the text of the short data service, in the coding that its DD format names.
  - location: positions in every format that radios report, and the NMEA position report of AnyTone radios to send.
  - lip: the ETSI Location Information Protocol that a UDT carries in the LIP format.
  - nmea: the NMEA 0183 RMC sentence that radios send as a position report in a data packet, to read and to write.
  - mototrbo/ars, mototrbo/lrrp, mototrbo/tms: the MOTOTRBO registration, location and text message services.
  - hytera/hdap: the Hytera DMR Application Protocol (HDAP) with its radio registration, location and text message services.

Networks and modems:

  - transport/dmrd: the protocols of the 53-octet DMRD packet: the MMDVM network protocol, the HomeBrew Repeater Protocol, the MMDVMHost to DMRGateway link and OpenBridge.
  - transport/dmrd/dmrdauth: the login token of transport/dmrd and the signatures of OpenBridge.
  - transport/dmrd/dmrdsession: the login, keepalive and OpenBridge sessions of transport/dmrd, without I/O.
  - transport/ipsc: Motorola IP Site Connect.
  - transport/ipsc/ipscauth: the authentication trailer of IPSC packets.
  - transport/ipsc/ipscsession: the registration, keepalive and peer list of IPSC, without I/O.
  - transport/hytera: Hytera IP Multi-Site Connect.
  - transport/mmdvm: the serial protocol of an MMDVM modem.

Radio and voice:

  - rf: a receiver that recovers bursts from the I/Q samples of a software radio.
  - cmd/dmrrf: a command that writes DMR test transmissions as I/Q or FM modulator audio and decodes DMR from I/Q or discriminator audio.
  - vocoder: the Codec interface and the AMBE+2 FEC of a voice slot.
  - vocoder/codec2: a Codec2 Codec (libcodec2 in WebAssembly, or the system libcodec2 with -tags codec2native).
  - vocoder/dvsi: the packet protocol of the DVSI AMBE-3000R and AMBE-3003 chips, and a Codec with such a chip.
  - privacy: opt-in privacy: the algorithm and key ID of an encrypted call or data packet, ARC4, AES and Basic Privacy keystreams for the AMBE+2 frames of a voice.Receiver and a voice.Transmitter, and ARC4 for the packets of a data.Receiver.

The C API of the library, libdmrgo, is in v2/capi, a module of its own. It has an ABI number of its own; the README gives its version rules.
<!-- /dmrgo:doc -->

## Air interface, layers 2 and 3

- [enums](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/enums)
- [constants](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/constants)
- [fec](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec)
- [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2)
- [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu)
- [layer2/packet](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet)
- [layer2/framer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/framer)
- [layer3](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer3)

## Services of a channel and of a site

- [voice](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice)
- [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data)
- [csbk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk)
- [bs](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs)
- [trunk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk)
- [trunk/trunkauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk/trunkauth)

## Applications in data packets

- [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip)
- [sms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms)
- [shortdata](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/shortdata)
- [location](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location)
- [lip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip)
- [nmea](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea)
- [mototrbo/ars](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars)
- [mototrbo/lrrp](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp)
- [mototrbo/tms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms)
- [hytera/hdap](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/hytera/hdap)

## Networks and modems

- [transport/dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd)
- [transport/dmrd/dmrdauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth)
- [transport/dmrd/dmrdsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession)
- [transport/ipsc](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc)
- [transport/ipsc/ipscauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth)
- [transport/ipsc/ipscsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession)
- [transport/hytera](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera)
- [transport/mmdvm](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm)

## Radio and voice

- [rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf)
- [cmd/dmrrf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/cmd/dmrrf)
- [vocoder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder)
- [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2)
- [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi)
- [privacy](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/privacy)

## C API

The C form of the library is in `v2/capi`. See [C API](../targets/c.md) and [C header](c-api.md).
