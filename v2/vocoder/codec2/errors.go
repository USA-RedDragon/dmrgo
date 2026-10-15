package codec2

import "errors"

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrUnavailable is returned when the package was built without
	// libcodec2 (CGO_ENABLED=0, -tags nocodec2 or TinyGo).
	ErrUnavailable = errors.New("codec2: built without libcodec2")
	// ErrUnsupportedMode is returned for a Mode other than Mode2400 or Mode3200.
	ErrUnsupportedMode = errors.New("codec2: unsupported mode")
	// ErrFrameLength is returned when Encode gets other than FrameSamples samples.
	ErrFrameLength = errors.New("codec2: wrong number of samples")
	// ErrClosed is returned when a closed Codec is used.
	ErrClosed = errors.New("codec2: codec is closed")
)
