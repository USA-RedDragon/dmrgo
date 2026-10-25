//go:build !nocodec2 && !tinygo && !codec2native && (!cgo || codec2wasm) && !(js && wasm) && !codec2aot && ((amd64 && (linux || darwin || freebsd || netbsd || windows || dragonfly || solaris || illumos)) || (arm64 && (linux || darwin || freebsd || netbsd || windows))) && !android && !ios

package codec2

const embeddedAOT = false
