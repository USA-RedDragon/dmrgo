# dmrgo

[![Go Reference](https://pkg.go.dev/badge/github.com/USA-RedDragon/dmrgo/v2.svg)](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) [![go.mod version](https://img.shields.io/github/go-mod/go-version/USA-RedDragon/dmrgo.svg?filename=v2/go.mod)](https://github.com/USA-RedDragon/dmrgo) [![coverage](https://raw.githubusercontent.com/USA-RedDragon/dmrgo/main/.github/badges/coverage-v2.svg)](https://github.com/USA-RedDragon/dmrgo/actions/workflows/test.yaml) [![License](https://badgen.net/github/license/USA-RedDragon/dmrgo)](https://github.com/USA-RedDragon/dmrgo/blob/main/LICENSE)

Decode and encode DMR (Digital Mobile Radio) bursts and PDUs as defined in [ETSI TS 102 361](https://www.etsi.org/standards) parts 1 to 4, run the calls, data, trunking and network protocols of DMR without I/O, and receive and transmit DMR on software radios and FM radios. The library runs on a PC, in the browser and on a microcontroller with TinyGo.

[DMRHub](https://github.com/USA-RedDragon/DMRHub) and [ipsc2mmdvm](https://github.com/USA-RedDragon/ipsc2mmdvm) use it.

## Installation

Requires Go 1.24 or later.

```sh
go get github.com/USA-RedDragon/dmrgo/v2
```

## Getting started

```go
package main

import (
	"fmt"
	"log"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func main() {
	// A Voice LC Header burst as received from a repeater.
	raw := [33]byte{
		68, 75, 3, 135, 36, 66, 12, 240, 21, 240,
		12, 161, 196, 109, 255, 87, 215, 93, 245, 222,
		49, 168, 53, 24, 63, 48, 61, 97, 56, 82,
		151, 134, 91,
	}

	burst, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		log.Fatal(err)
	}
	if err := burst.FEC.Aggregate().Err(); err != nil {
		log.Fatal(err)
	}
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
		target, source, _ := lc.Addresses()
		fmt.Printf("%s: call from %d to %d\n", burst.SlotType.DataType, source, target)
	}

	encoded, err := burst.Encode()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("re-encoded unchanged:", encoded == raw)
}
```

The program prints `Voice LC Header: call from 3191868 to 9990` and `re-encoded unchanged: true`.

An FEC or CRC error does not make `NewBurstFromBytes` fail. `burst.FEC` reports it, and `burst.Data` still holds the decoded PDU. Change a field of `burst.Data` and `Encode` writes a new burst with new parity. Fields of bits are packed in bytes in transmit order, with the first bit on air in the most significant bit of byte 0. A burst is a `[33]byte`, a CSBK a `[12]byte` and a vocoder slot a `[9]byte`.

The [package documentation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) has the package map and the errors to match. [Examples](docs/reference/examples.md) lists the example programs, which run offline on captures.

## Features

[Features](docs/reference/features.md) gives the full list.

- Bursts and PDUs (`layer2`, `layer2/pdu`): decode, encode and build every burst and PDU of the air interface, with in-place field access, voice superframes, the embedded and Short LC assemblers and the MMDVMHost burst check.
- Packet data (`layer2/packet`, `data`): confirmed and unconfirmed packets at rate 1/2, 3/4 and 1, with selective retry, sliding windows and the applications that radios carry in them: IP, text messages, positions, LIP, and the MOTOTRBO and Hytera services.
- Calls (`voice`, `csbk`, `privacy`): voice call receive and transmit with late entry, channel access and hang time; CSBK services and TDMA direct mode timing; ARC4, AES and Basic Privacy, opt in.
- Base station and trunking (`bs`, `trunk`): a two-slot repeater or hotspot, and Tier III trunking for the MS, a scanner and a single-site TSCC.
- Network protocols (`transport/...`): the MMDVM network protocol, HBRP, OpenBridge, Motorola IPSC, Hytera IPMSC and the MMDVM serial protocol, with session machines and their authentication.
- Radio (`rf`): a receiver of I/Q and FM discriminator audio in four tiers, from a microcontroller to a PC ([Receiver](docs/rx/README.md)), and a transmitter of I/Q and modulator audio that accepts only setups measured to keep the limits of a stated region and service ([Transmitter](docs/tx/README.md)).
- Vocoders (`vocoder`): the AMBE+2 frame FEC, Codec2, and AMBE+2 on a DVSI chip ([Transcode voice](docs/guides/transcode-voice.md)).
- FEC: every DMR code and CRC, each with one result type.
- C API (`v2/capi`): libdmrgo, a shared or static library with the whole API in C.
- No allocation on the hot path, with gc and TinyGo ([Allocation](docs/concepts/allocation.md), [Microcontrollers](docs/targets/mcu.md)).

## Documentation

- [Package reference](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2): the Go API of every package, on pkg.go.dev.
- [Features](docs/reference/features.md): every feature, by package.
- [Examples](docs/reference/examples.md): the example programs.
- [Receiver](docs/rx/README.md): the tiers of `rf`, their sensitivity and cost, and how they are measured.
- [Transmitter](docs/tx/README.md): the tiers, formats and rates, the regional limits and the measured margins.
- [Transcode voice](docs/guides/transcode-voice.md): AMBE+2 FEC, the Codec2 backends and DVSI chips.
- [Allocation](docs/concepts/allocation.md): where the library can allocate, and the checks.
- Targets: [PC](docs/targets/pc.md), [Browser](docs/targets/browser.md), [Microcontrollers](docs/targets/mcu.md), [C API](docs/targets/c.md) and [GNU Radio](docs/targets/gnuradio.md).
- [Go API](docs/reference/go-api.md) and [C header](docs/reference/c-api.md): the packages and the C declarations.
- [Tools](docs/reference/tools.md): dmrrf and the commands that build the library and its docs.
- [Tooling](docs/project/tooling.md): the generators and the checks of the generated pages.
- [ETSI coverage](docs/reference/etsi.md): the status of each clause of ETSI TS 102 361.
- [Versioning](docs/project/versioning.md), [v2.1 breaking changes](docs/project/breaking-v2.1.md) and [License](docs/project/license.md).

## Versioning

- The `v2` module follows semantic versioning. A new package, function, method, struct field, constant or enum value comes in a minor release. A change that breaks code that compiles comes only in a new major version.
- A minor release can add a value to an enum type, so give a switch over an enum a default case.
- The minimum Go version rises only in a minor release, with a note in the release notes.
- libdmrgo, the C API, has its own ABI number (`DMRGO_ABI_VERSION`) and soname (`libdmrgo.so.N`). It is built from the `v2` tag, and `v2/capi` has no tags of its own. A change that breaks C code that compiles against `dmrgo.h`, or a program that links an older libdmrgo, gives a new ABI number and soname. v2.0.0 to v2.0.3 have ABI 1, v2.0.4 and v2.0.5 have ABI 2, v2.0.6 to v2.0.8 have ABI 3, and the release after v2.0.8 has ABI 4 (`libdmrgo.so.4`).
- A config, options or vtable struct of the C API whose first member is `size_t size` is sized. The caller sets `size` to `sizeof` the struct, and the library reads only the members that `size` covers and takes the rest as zero. Thus a minor release can add members at the end of such a struct without a new ABI number, and a program built against older headers works with the newer library. Other structs have a fixed layout.
