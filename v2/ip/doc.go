// Package ip decodes and encodes the UDP datagrams that DMR data packets
// carry: an IPv4 header and a UDP header with the SAP IP based packet data,
// or the compressed UDP/IPv4 header with the SAP UDP/IP header compression.
// It also decodes and encodes the ICMP error messages that report a packet
// that could not be delivered, maps radio and group IDs to IPv4 addresses,
// and names the UDP ports of the application protocols. It does no I/O and
// does not allocate except to grow a slice of the caller. (ETSI TS 102
// 361-3 clauses 5.1, 5.2, 5.5 and 5.6)
package ip
