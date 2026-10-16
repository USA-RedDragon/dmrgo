// Package codec2 implements vocoder.Codec with the Codec2 speech codec: it
// encodes 8 kHz PCM into 72-bit DMR vocoder frames and decodes them, in the
// DMR framing that QRadioLink uses.
//
// By default the package binds the system libcodec2 through cgo, so a build
// that imports it needs a C compiler and libcodec2 with its pkg-config file
// (pkg-config name codec2), for example the libcodec2-dev package on Debian.
// Without them the build fails with "Package 'codec2' not found". Build with
// -tags nocodec2 or CGO_ENABLED=0 to remove the dependency; New then returns
// ErrUnavailable. TinyGo builds always use this path.
//
// Each 20 ms Codec2 frame (160 samples at 8 kHz) fills one 72-bit DMR
// vocoder frame, so a 60 ms voice burst carries three. The Codec2 bytes go
// MSB first at the start of the 9-byte frame and the remaining bits are zero.
// There is no FEC, scrambling or interleaving. Calls are marked with the
// Feature set ID 0xC2 in the Full LC. The FID does not identify the Codec2
// mode, so both ends must agree on it out of band.
//
// The package does not register itself. A program that creates codecs by
// name registers it once, before vocoder.New:
//
//	if err := vocoder.Register(codec2.Name, codec2.NewFactory(codec2.Config{})); err != nil {
//		return err
//	}
//
// libcodec2 is licensed under the GNU LGPL 2.1. This package links it
// dynamically and contains none of its source. A program that links
// libcodec2 statically must meet the LGPL terms for that copy.
package codec2
