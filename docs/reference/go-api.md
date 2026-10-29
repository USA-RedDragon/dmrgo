# Go API

This page links the documentation of each public package of the `v2` module on pkg.go.dev, in the groups of the package overview.

The root package [dmrgo](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) holds the overview of the module, the errors that every package shares and the types that several packages use. Its package doc lists the packages:

<!-- dmrgo:doc pkg=. section=Packages -->
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
