// Package ars decodes and encodes the messages of the MOTOTRBO Automatic
// Registration Service: the registrations that radios send in UDP
// datagrams to port 4005, and the answers and queries of the server. The
// layout comes from open implementations; Motorola does not publish it. It
// does no I/O and does not allocate except to grow a slice of the caller.
package ars
