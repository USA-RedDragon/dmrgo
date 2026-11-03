//go:build !tinygo

package rf

import "sync/atomic"

var audioRx atomic.Bool //nolint:gochecknoglobals // set by the audio constructor, so builds without one drop the audio-only code

func setAudioOn() { audioRx.Store(true) }

func audioOn() bool { return audioRx.Load() }
