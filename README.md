# dmrgo

This library decodes a raw 33-byte DMR packet into Golang-native structures.

## Usage

A complete example can be found at `examples/benchmark.go`

The full definitions of `EXAMPLE_DMR_PARROT_KERCHUNK_BURSTS`
and `EXAMPLE_DMR_SMS_BURSTS` are excluded for brevity.

```go
const (
    DMR_BURST_SIZE = 33

    DMR_PARROT_KERCHUNK_BURST_COUNT = 15
    DMR_SMS_BURST_COUNT             = 8
)

var EXAMPLE_DMR_PARROT_KERCHUNK_BURSTS = [DMR_PARROT_KERCHUNK_BURST_COUNT][DMR_BURST_SIZE]byte{
    {68, 75, 3, 135, 36, 66, 12, 240, 21, 240, 12, 161, 196, 109, 255, 87, 215, 93, 245, 222, 49, 168, 53, 24, 63, 48, 61, 97, 56, 82, 151, 134, 91},           // First, burst
    ...
}

var EXAMPLE_DMR_SMS_BURSTS = [DMR_SMS_BURST_COUNT][DMR_BURST_SIZE]byte{
    {66, 74, 30, 185, 189, 0, 34, 4, 51, 201, 3, 83, 4, 205, 255, 87, 215, 93, 245, 218, 200, 246, 69, 24, 215, 120, 191, 128, 181, 29, 41, 17, 231},
    ...
}

func main() {
    for _, rawBurst := range EXAMPLE_DMR_PARROT_KERCHUNK_BURSTS {
        timeStarted := time.Now()
        burst := layer2.NewBurstFromBytes(rawBurst)
        timeElapsed := time.Since(timeStarted)
        fmt.Printf("%s: %v\n", enums.SyncPatternToName(burst.SyncPattern), burst.ToString())
        fmt.Printf("Took %s to decode burst\n", timeElapsed)
    }

    fmt.Println("---------------------------------")

    for _, rawBurst := range EXAMPLE_DMR_SMS_BURSTS {
        timeStarted := time.Now()
        burst := layer2.NewBurstFromBytes(rawBurst)
        timeElapsed := time.Since(timeStarted)
        fmt.Printf("%s: %v\n", enums.SyncPatternToName(burst.SyncPattern), burst.ToString())
        fmt.Printf("Took %s to decode burst\n", timeElapsed)
    }
}
```

## Thanks To

<https://github.com/pd0mz/go-dmr> for several of the checksum and error correction sources.
