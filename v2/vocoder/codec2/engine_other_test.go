//go:build !nocodec2 && !tinygo && (codec2native || (js && wasm))

package codec2

import "testing"

func testModule(*testing.T) *Module {
	return nil
}

func allocModules(*testing.T) []*Module {
	return []*Module{nil}
}
