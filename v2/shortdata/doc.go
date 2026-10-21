// Package shortdata decodes and encodes the data of the DMR short data
// service: the text of a defined data packet in the character coding that
// its DD format names, and the H-SMS text that AnyTone radios send as
// defined data. It does no I/O and does not allocate except to grow a
// slice of the caller. (ETSI TS 102 361-3 clause 6)
package shortdata
