package codec2_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2"
)

// ExampleNew creates a Codec2 codec, and also runs in a build without
// libcodec2, where New returns ErrUnavailable.
func ExampleNew() {
	c, err := codec2.New(&codec2.Config{Mode: codec2.Mode2400})
	if errors.Is(err, codec2.ErrUnavailable) {
		fmt.Println(codec2.FrameSamples, "samples per 20 ms frame")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()
	fmt.Println(c.FrameSamples(), "samples per 20 ms frame")
	// Output:
	// 160 samples per 20 ms frame
}
