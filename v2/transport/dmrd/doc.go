// Package dmrd encodes and decodes the UDP protocols built around the
// 53-octet DMRD packet: the MMDVM network protocol of MMDVMHost and
// DMRGateway (DialectMMDVM), which BrandMeister, DMR+, HBlink, FreeDMR and
// DMRHub masters speak; the HomeBrew Repeater Protocol in the hex text form
// of its 2015 specification and of BrandMeister (DialectHBRP); the link
// between MMDVMHost and DMRGateway on one host (DialectLocal); and OpenBridge
// between masters. It does no IO and keeps no state.
//
// The DMRD packet is the same in every dialect: DecodeData and
// Data.AppendBinary take no dialect. The login, keepalive and other control
// messages differ, so DecodeControl and Control.AppendBinary take one, as
// Classify does. Data.Hint gives the layer2.Hint for the service machines.
//
// Decode functions never allocate and never panic. They return the static
// errors ErrShort, ErrLength, ErrSignature, ErrText and ErrCommand, which
// errors.Is matches. AppendBinary methods allocate only to grow dst. The
// login token and the OpenBridge signatures are plain octets in the
// messages; package auth computes and checks them.
//
// (DL5DI, G4KLX and DG1HT, IPSC Protocol Specs for homebrew DMR repeater,
// 2015-07-26; MMDVMHost 9384d68 and DMRGateway ee79d38; BrandMeister Open
// Bridge; FreeDMR 546165eb)
package dmrd
