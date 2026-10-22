package location

import "errors"

// ErrNoFix is returned for a data packet that does not carry a position in
// a format that the package knows. Match it with errors.Is. Decode also
// returns the errors of the ip, lrrp, nmea and hdap packages.
var ErrNoFix = errors.New("location: packet carries no position")

// ErrNoPosition is returned with the Fix for a location report or answer
// that has no position, such as the answer of a radio without a GNSS fix.
// The Fix holds the Format, the addresses, the Result and RequestID of the
// answer, and the other fields that the message has. Match it with
// errors.Is.
var ErrNoPosition = errors.New("location: report carries no position")
