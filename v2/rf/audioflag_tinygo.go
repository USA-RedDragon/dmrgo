//go:build tinygo

package rf

var audioRx bool //nolint:gochecknoglobals // set by the audio constructor, so builds without one drop the audio-only code

func setAudioOn() { audioRx = true }

func audioOn() bool { return audioRx }
