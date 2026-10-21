// Package dmrgo decodes and encodes DMR (Digital Mobile Radio) bursts and
// PDUs. This package holds only the module overview. The code is in the
// packages listed below. (ETSI TS 102 361 parts 1 to 4)
//
// # Getting started
//
// A DMR burst is 264 bits (33 bytes). Decode one with
// layer2.NewBurstFromBytes:
//
//	burst, err := layer2.NewBurstFromBytes(raw)
//	if err != nil {
//		return err
//	}
//	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok && !burst.FEC.PDU.Uncorrectable {
//		target, source, _ := lc.Addresses()
//		fmt.Println(source, target)
//	}
//
// An FEC or CRC failure is not an error. Burst.FEC reports it, and Burst.Data
// still holds the decoded PDU. Call Err on a fec.Result, such as
// burst.FEC.Aggregate().Err(), to get fec.ErrUncorrectable instead.
//
// Burst.Encode writes a decoded burst back to 33 bytes, unchanged until its
// fields change. Burst.Data is a layer2.Data: a PDU whose DataType must match
// SlotType.DataType. For a Full LC that is its BurstDataType, which selects
// the RS(12,9) mask.
//
// layer2.RawBurst reads and writes single fields of a 33-byte burst in place,
// without a full decode: the SYNC pattern, slot type, BPTC, rate 3/4 and rate
// 1 payloads, EMB, embedded data and voice slots.
//
// After construction (NewBurstFromBytes, NewVoiceTransmitter, framer.New,
// packet.NewAssembler and the like), the hot path allocates nothing: decode,
// encode, build, receive and assemble calls, with gc and with TinyGo at the
// default stack size of a microcontroller target. Errors are static values,
// so a call that fails does not allocate either. A call that returns a fresh
// slice, string or payload struct allocates it, and its doc says so. Each has
// a form that writes into storage of the caller, such as pdu.DecodeCSBKInto
// for pdu.DecodeCSBK and the Append forms. A layer2.Burst keeps its decoded
// Data and the payload structs that Data points to until the next decode
// into it, so it must not be copied after its first decode. The README lists
// every place where the library can allocate, and the tests in
// internal/allocgate and internal/allocs check it.
//
// The PDUs of package pdu decode with pdu.DecodeX and encode with the Encode
// method of the PDU. A PDU with its own FEC or CRC, such as a CSBK, Full LC,
// data header or slot type, reports the result in its FEC field.
//
// Every field of bits that the library takes or returns is packed in bytes
// in transmit order: the first bit on air is the most significant bit of
// byte 0. A 264-bit burst is a [33]byte, a 96-bit PDU a [12]byte and a
// 72-bit vocoder slot a [9]byte. When a field is not a whole number of
// bytes, the low bits of the last byte are padding: a decode ignores them
// and an encode sets them to 0. An encoder that takes such a field, such as
// CACH.Encode, UnifiedSingleBlockData.Encode or vocoder.EncodeAMBE72,
// returns a *FieldRangeError when a padding bit is set, as it does for a
// value that does not fit its bits. So do the setters, which then change
// nothing, the SetAddresses methods for an address over 24 bits, and the
// functions and getters that read a value of a fixed width, such as
// pdu.AddressHash, SysCode.Unpack14 and ShortLCCSysParms.NetID.
//
// # Packages
//
// Air interface, layers 2 and 3:
//
//   - enums: the enumerated information elements, such as data types, SYNC
//     patterns, FLCO, FID and color codes.
//   - constants: the timers and constants of the Annexes of ETSI TS 102 361.
//   - fec: the FEC result that decoders return, and ErrUncorrectable.
//   - layer2: bursts, the byte-level RawBurst, the embedded LC, CACH and
//     Short LC, the Reverse Channel, and the voice superframe transmitter
//     and receiver.
//   - layer2/pdu: the layer 2 PDUs (CSBK, Full LC, Short LC, data headers,
//     MBC, PI header, slot type, EMB and data blocks) and their byte-level
//     editors.
//   - layer2/packet: segmentation and reassembly of data packets, with
//     confirmed delivery and response packets.
//   - layer2/framer: finds bursts in a demodulated bit stream.
//   - layer3: the layer 3 information elements that PDUs carry, such as
//     service options and channel definitions.
//
// Services of a channel and of a site:
//
//   - voice: the voice calls of one logical channel: a Receiver that turns
//     bursts into call events, and a Transmitter.
//   - data: data calls on one logical channel: a Receiver that delivers
//     packets and answers confirmed ones, and a Sender with channel access
//     and retries.
//   - csbk: the CSBK services of one logical channel, without I/O: a
//     request with its answer and retries, the answers of a radio,
//     Pre_CSBKs and the wakeup of a BS.
//   - bs: the base station of a two-slot carrier: a repeater, a hotspot or
//     the slot arbiter of a network server.
//   - trunk: the Tier III channel tables, a Follower that follows the
//     calls of a trunked site, an MS that registers and makes calls, and a
//     TSCC that runs a single site.
//
// Applications in data packets:
//
//   - ip: the IPv4 and UDP headers of data packets, and the compressed
//     UDP/IPv4 header.
//   - sms: text messages in every format that radios use.
//   - shortdata: the text of the short data service, in the coding that its
//     DD format names.
//   - location: positions in every format that radios report.
//   - nmea: the NMEA 0183 RMC sentence that radios send as a position
//     report in a data packet.
//   - mototrbo/ars, mototrbo/lrrp, mototrbo/tms: the MOTOTRBO registration,
//     location and text message services.
//   - hytera: the Hytera DMR Application Protocol (HDAP) with its radio
//     registration, location and text message services.
//
// Networks and modems:
//
//   - transport/dmrd: the protocols of the 53-octet DMRD packet: the MMDVM
//     network protocol, the HomeBrew Repeater Protocol, the MMDVMHost to
//     DMRGateway link and OpenBridge.
//   - transport/dmrd/auth: the login token of transport/dmrd and the
//     signatures of OpenBridge.
//   - transport/ipsc: Motorola IP Site Connect.
//   - transport/ipsc/auth: the authentication trailer of IPSC packets.
//   - transport/hytera: Hytera IP Multi-Site Connect.
//   - transport/mmdvm: the serial protocol of an MMDVM modem.
//
// Voice:
//
//   - vocoder: the Codec interface and the AMBE+2 FEC of a voice slot.
//   - vocoder/codec2: a Codec2 Codec (needs cgo and libcodec2).
//   - vocoder/dvsi: the packet protocol of the DVSI AMBE-3000R and
//     AMBE-3003 chips, and a Codec with such a chip.
//
// The C API of the library, libdmrgo, is in v2/capi, a module of its own.
//
// # Errors
//
// These errors can be matched with errors.Is. Each is a static value.
//
//   - fec.ErrUncorrectable: an FEC or CRC check failed. It is wrapped by
//     layer2.ErrEmbeddedLCUncorrectable, packet.ErrMessageCRC and
//     packet.ErrBlocksMissing, which layer2.VoiceReceiver and
//     packet.Assembler return together with what they decoded, and by
//     packet.ErrHeaderCRC, which packet.Assembler returns for a data header
//     that it drops.
//   - layer2.ErrReservedDataType, layer2.ErrColorCodeMismatch: returned by
//     the burst decoders.
//   - layer2.ErrVoiceTerminated, layer2.ErrEmbeddedLCSequence,
//     layer2.ErrShortLCSequence, layer2.ErrShortLCIncomplete: returned by the
//     voice transmitter and receiver and the Short LC assembler.
//   - layer2.ErrDataType, layer2.ErrSyncSource, layer2.ErrVoicePosition:
//     returned, some wrapped, by the burst builders and by the SetKind and
//     SetRawBurst methods of the transports for a data type, sync source or
//     voice position that the call cannot take.
//   - packet.ErrNoHeader, packet.ErrRateChange, packet.ErrRate,
//     packet.ErrFormat, packet.ErrTooLong, packet.ErrResponseData,
//     packet.ErrUnexpectedRetry, packet.ErrNoFirstTry,
//     packet.ErrNoRetryBlocks: returned by packet segmentation and
//     reassembly.
//   - pdu.ErrTalkerAliasIncomplete, pdu.ErrTalkerAliasLength,
//     pdu.ErrTalkerAliasInvalidUTF8: returned by the talker alias functions.
//   - pdu.ErrCapacityPlusSequence, pdu.ErrCapacityPlusIncomplete: returned
//     by the Capacity Plus site status assembler.
//   - pdu.ErrNoAddresses: returned by the SetAddresses methods for a PDU
//     without address fields.
//   - voice.ErrBusy, voice.ErrQueueFull, voice.ErrNotActive,
//     voice.ErrTimedOut: returned by voice.Transmitter.
//   - data.ErrBusy: returned by data.Sender.Send while a message is in
//     progress.
//   - csbk.ErrBusy, csbk.ErrNoAddresses: returned by the requests of the
//     csbk machines.
//   - bs.ErrSlot: returned for a slot other than 1 or 2.
//   - trunk.ErrChannelNumber, trunk.ErrPlanFull, trunk.ErrBusy,
//     trunk.ErrNotOnControl, trunk.ErrNotRegistered: returned by the
//     channel tables and the requests of the trunk machines.
//   - ip.ErrShort, ip.ErrLength, ip.ErrVersion, ip.ErrProtocol,
//     ip.ErrFragment, ip.ErrSAP, ip.ErrChecksum: returned by the ip
//     decoders.
//   - sms.ErrShort, sms.ErrLength, sms.ErrSignature, sms.ErrNotText,
//     sms.ErrFormat: returned by package sms, which also returns the
//     errors of the ip, tms and shortdata packages.
//   - shortdata.ErrShort, shortdata.ErrLength, shortdata.ErrFormat,
//     shortdata.ErrCharacter: returned by package shortdata.
//   - location.ErrNoFix: returned for a packet without a position. Package
//     location also returns the errors of the ip, lrrp and nmea packages.
//   - nmea.ErrSentence, nmea.ErrChecksum, nmea.ErrField: returned by
//     nmea.ParseRMC.
//   - ars.ErrShort, ars.ErrLength, tms.ErrShort, tms.ErrLength,
//     lrrp.ErrShort, lrrp.ErrLength, lrrp.ErrToken, lrrp.ErrVarint:
//     returned by the MOTOTRBO decoders.
//   - hytera.ErrShort, hytera.ErrLength, hytera.ErrSignature,
//     hytera.ErrText: returned by the HDAP decoders.
//   - dmrd.ErrShort, dmrd.ErrLength, dmrd.ErrSignature, dmrd.ErrText,
//     dmrd.ErrCommand: returned by the transport/dmrd decoders.
//   - ipsc.ErrShort, ipsc.ErrLength, ipsc.ErrCommand: returned by the
//     transport/ipsc decoders.
//   - ErrShort, ErrLength, ErrSignature and ErrCode of transport/hytera:
//     returned by its decoders.
//   - mmdvm.ErrShort, mmdvm.ErrLength, mmdvm.ErrCommand: returned by the
//     transport/mmdvm decoders.
//   - vocoder.ErrUnknownCodec, vocoder.ErrDuplicateCodec,
//     vocoder.ErrNilFactory, vocoder.ErrRegistryFull: returned by the
//     codec registry.
//   - codec2.ErrUnavailable, codec2.ErrUnsupportedMode,
//     codec2.ErrFrameLength, codec2.ErrClosed: returned by the Codec2
//     Codec.
//   - dvsi.ErrIncomplete, dvsi.ErrStart, dvsi.ErrType, dvsi.ErrLength,
//     dvsi.ErrParity, dvsi.ErrField, dvsi.ErrChannel, dvsi.ErrSamples,
//     dvsi.ErrBits, dvsi.ErrStatus, dvsi.ErrTimeout, dvsi.ErrPort,
//     dvsi.ErrClosed: returned by the DVSI packet parser and Codec.
//
// The auth packages export no errors. They return an error for a dialect,
// key or packet that they do not take.
//
// These error types can be matched with errors.As:
//
//   - *FieldRangeError: an encoder, setter, getter or function got a value
//     that does not fit in its bits, or a field with a padding bit set.
//     enums.NewColorCode returns it for a value over 15.
//   - *ConfigError: a constructor, or a machine method of bs, csbk, data,
//     trunk or voice, got a config, option or argument field out of range.
//     Type names the package.
//
// # More
//
// The README lists the example programs and the ETSI coverage of each
// clause: https://github.com/USA-RedDragon/dmrgo.
package dmrgo
