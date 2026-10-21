// Package tms decodes and encodes the messages of the MOTOTRBO Text
// Messaging Service: the text messages, acknowledgements and service
// availability messages that radios send in UDP datagrams to port 4007.
// AnyTone radios send it in their M-SMS mode. The layout comes from radio
// captures and open implementations; Motorola does not publish it. It does
// no I/O and does not allocate except to grow a slice of the caller.
package tms
