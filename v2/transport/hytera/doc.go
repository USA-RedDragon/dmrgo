// Package hytera encodes and decodes the UDP packets of Hytera IP
// Multi-Site Connect as Hytera repeaters send them: the P2P service
// (registration, service start, redirect and heartbeat) and the DMR service
// (one 72-octet frame per burst, and a 1-octet keepalive). It does no IO
// and keeps no state. The RDAC service and the Hytera application
// protocols (HRNP, HSTRP, HDAP) are not here.
//
// Hytera publishes no specification. The layouts follow a real capture
// where the sources disagree, and every field whose meaning no source
// proves is kept as received, so a decode followed by an encode gives the
// same octets.
//
// Decode functions never allocate and never panic. They return the static
// errors ErrShort, ErrLength, ErrSignature and ErrCode, which errors.Is
// matches.
//
// (OK-DMR dmr-kaitai 7489ff1 ip_site_connect_protocol.ksy and
// ip_site_connect_heartbeat.ksy; OK-DMR Hytera_Homebrew_Bridge 05e1bf9 and
// its captures do1kbl.pcapng and gw_hytera_mmdvm_*.pcapng)
package hytera
