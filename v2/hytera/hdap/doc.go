// Package hdap decodes and encodes the messages of the Hytera DMR
// Application Protocol (HDAP): the frame with its checksum, and the
// payloads of the radio registration service (RRS), the location protocol
// (LP) and the text message protocol (TMP). Hytera radios exchange them
// with a PC on UDP ports 3002 to 3009. The layout comes from open
// implementations; Hytera does not publish it, and no capture of a Hytera
// radio has checked it. It does no I/O and does not allocate except to
// grow a slice of the caller.
package hdap
