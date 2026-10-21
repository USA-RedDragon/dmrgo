// Package lrrp decodes and encodes the messages of the MOTOTRBO Location
// Request and Response Protocol: the location requests that a server
// sends, and the positions that radios report, in UDP datagrams to port
// 4001. Hytera radios send it too. A message is a document of tokens in
// MOTOTRBO binary XML. The layout comes from open implementations and the
// examples of the Motorola specification, which Motorola does not publish.
// It does no I/O and does not allocate except to grow a slice of the
// caller.
package lrrp
