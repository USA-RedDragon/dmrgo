//go:build codec2native && !cgo && !nocodec2 && !tinygo

package codec2

var _ int = "codec2: -tags codec2native needs cgo and libcodec2"
