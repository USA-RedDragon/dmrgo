# v2.1 breaking changes

This page lists the changes after v2.0.8 that break Go code or C code that builds against v2.0.8, and how to update your code.

The list comes from `apidiff -m -incompatible` between the v2.0.8 tag and main, and from the lines that left `v2/capi/abi.txt` when the C ABI went from 3 to 4. Each item names the commit that made the change.

## Go API

| Change | Update your code | Commit |
| --- | --- | --- |
| [csbk.Requester.Request](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Request) takes a `*pdu.CSBK` in place of a `pdu.CSBKBytes`. | Pass the CSBK that you built, without `Encode`. `Request` sets `LastBlock` on its copy and does not change your CSBK. | b9dccc1fc |
| `csbk.Output.Target`, `Source` and `Group` and `data.Output.Group` are gone. | Read [layer2.Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Call) from the new `Call` field of the output. `voice.Output` has the same field. | e3d906594 |
| `transport/dmrd.Data.Private` is gone. | Use [dmrd.Data.Group](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data), which is true for a group call: the inverse of `Private`. | eadc5f883 |
| [layer2.EncodeEmbeddedLC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#EncodeEmbeddedLC), [layer2.DecodeEmbeddedLC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#DecodeEmbeddedLC) and [layer2.EmbeddedLCAssembler.Add](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#EmbeddedLCAssembler.Add) take and return a [pdu.LCBytes](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#LCBytes) in place of a `[9]byte`. | Convert with `pdu.LCBytes(b)` and `[9]byte(lc)`, or read the fields with the methods of `pdu.LCBytes`, such as `FLCO` and `Call`. | 679d2d495 |
| [voice.Transmitter.Stop](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Stop) takes the current time. | Call `Stop(now)` with the same clock that you give to `Tick`. | 38ca85a09 |
| `privacy.Service` and its constants are now an alias of [dmrgo.Service](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#Service), which `rf` also uses. | Code that names `privacy.Service` and its constants still builds. Reflection and the `%T` verb of `fmt` give the type as `dmrgo.Service`. | 329a0b5b1 |
| With cgo on, [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2) uses the WebAssembly backend by default, and `codec2.Backend` is `"wasm"` in such a build. | Build with `-tags codec2native` to keep the system libcodec2. `codec2wasm` is accepted and changes nothing. | 072a27f57 |

## C API (ABI 4)

The C ABI number went from 3 to 4, and the soname from `libdmrgo.so.3` to `libdmrgo.so.4`. Rebuild your program against the new headers. These declarations of ABI 3 changed:

| Change | Commit |
| --- | --- |
| `dmrgo_csbk_requester_request` takes a `const dmrgo_pdu_csbk_t *` in place of a `const dmrgo_pdu_csbk_bytes_t *`. | b9dccc1fc |
| `dmrgo_csbk_output_t` has no `target`, `source` and `group` members, and `dmrgo_data_output_t` has no `group` member. These two structs and `dmrgo_voice_output_t` have a new `call` member, so their layout changed. | e3d906594 |
| `dmrgo_dmrd_data_t` has `group` in place of `private_`. | eadc5f883 |
| `dmrgo_layer2_encode_embedded_lc`, `dmrgo_layer2_decode_embedded_lc` and `dmrgo_layer2_embedded_lc_assembler_add` take or give a `dmrgo_pdu_lc_bytes_t` in place of `uint8_t[9]`. | 679d2d495 |
| `dmrgo_voice_transmitter_stop` takes `now`. | 38ca85a09 |
| `dmrgo_privacy_service_t` and `dmrgo_privacy_service_string` are gone. Use `dmrgo_service_t` and `dmrgo_service_string`. The `DMRGO_PRIVACY_SERVICE_*` constants and the `service` member of `dmrgo_privacy_transmitter_config_t` have the type `dmrgo_service_t`, and `DMRGO_SERVICE_*` have the same values. | 329a0b5b1 |
