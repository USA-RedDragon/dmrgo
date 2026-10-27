// Package privacy adds opt-in privacy to the voice calls of package voice
// and the data packets of package data: it reports whether a call is
// encrypted, with which algorithm and key ID, also without the key, and with
// the key it removes or applies the privacy of the AMBE+2 frames. Nothing
// else in the library uses this package, so a program that does not import
// it sends and receives voice as before.
//
// ETSI TS 102 361 defines only the containers: the PI header burst after the
// Voice LC Header or alone (ETSI TS 102 361-1 clauses 5.1.2.2, 6.2 and
// 9.3.6, CRC mask in Annex B.3.12) and the Privacy bit of the Service
// Options (ETSI TS 102 361-2 clause 7.2.1, table 7.11), with "Privacy is not
// defined in the present document". The DMR Association names the algorithms
// of its interoperable scheme (ARC4 40-bit, DES, AES-128 and AES-256) but
// does not publish the scheme. The framing here follows the public
// description at gophertrunk.org/reference/dmr-encryption, cross-checked
// with the open-source decoder dsd-fme, and the tests prove it on every
// frame of public captures with published keys:
//
//   - A PI header with FID 0x10 carries the algorithm ID, the key ID and a
//     32-bit message indicator (MI).
//   - Each superframe of 18 AMBE+2 frames has its own keystream. Frame k
//     of the superframe XORs keystream octets 7k to 7k+6 onto its 49 bits.
//   - ARC4 (Enhanced Privacy) keys RC4 with the 5 key octets and the 4 MI
//     octets and drops the first 256 octets. The MI of the next superframe
//     is 32 steps of the LFSR x^32 + x^4 + x^2 + 1.
//   - AES-128 and AES-256 run in OFB mode (NIST SP 800-38A clause 6.4) from
//     a 128-bit IV that the LFSR with taps 32, 22, 2 and 1 grows from the
//     MI, and drop the first block. The MI of the next superframe is octets
//     4 to 7 of the IV.
//   - Basic Privacy has no PI header: a 48-bit pattern from a 16-bit key
//     XORs every frame.
//
// A Receiver reads the events of a voice.Receiver and returns the frames of
// each voice burst in the clear. Its Data method removes ARC4 privacy from a
// packet of a data.Receiver whose P_HEAD (ETSI TS 102 361-1 clause 9.2.9) is
// a privacy header with FID 0x10: the payload, without the pad octets and
// the message CRC, XORs the same RC4 keystream as the first superframe of a
// voice call. It reports AES and Basic Privacy data but leaves it as it is,
// as no public capture with a key proves their framing. A Transmitter gives
// the PI header for voice.CallOptions and applies the privacy to the bursts
// of a voice.Transmitter. Its Data method applies ARC4 privacy to the
// payload of a data packet and gives its privacy header, the P_HEAD that
// data.Sender sends after the first header; the tests prove it byte for
// byte against the public capture. It refuses AES, DES and Basic Privacy
// data with ErrDataAlgorithm. Neither recovers the MI of a call that it joins
// after its PI header (late entry), and a Transmitter sends no late entry
// information.
//
// NewTransmitter takes the radio service of the station and refuses the
// amateur service with ErrAmateur, so that neither voice nor data is
// encrypted there: amateur rules forbid messages encoded to
// obscure their meaning (ITU Radio Regulations Article 25.2A, and for
// example 47 CFR 97.113(a)(4) in the United States). A Receiver reports and
// decodes encrypted calls in every service. Receiving encrypted traffic that
// you are not a party to can be unlawful, for example under 18 USC 2511 in
// the United States.
package privacy
