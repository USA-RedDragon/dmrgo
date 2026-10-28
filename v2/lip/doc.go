// Package lip decodes and encodes the PDUs of the Location Information
// Protocol (LIP) of ETSI TS 100 392-18-1, which a DMR UDT carries in the
// LIP format and the USBD polling service compresses. DecodeHeader gives
// the kind of a PDU. The package reads and writes the LONG LOCATION REPORT,
// the LOCATION REPORT ACKNOWLEDGEMENT, the IMMEDIATE LOCATION REPORT
// REQUEST and the LOCATION REPORTING ENABLE/DISABLE request and response,
// and the type 5 information elements of every PDU with NextElement and
// AppendElement. trunk.DecodeLIPShortReport reads the SHORT LOCATION
// REPORT. A PDU is a string of bits, the first in the most significant bit
// of the first octet; the encoders give the octets and the number of bits,
// with 0 bits after the last one. Information element values that the
// spec codes in steps, such as velocities and accuracies, stay codes. It
// does no I/O and does not allocate except to grow a slice of the caller.
// (ETSI TS 102 361-4 clause B.3.10, ETSI TS 100 392-18-1 V1.7.2 clause 6)
package lip
