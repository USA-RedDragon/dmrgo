package location

import "errors"

// ErrNoFix is returned for a data packet that does not carry a position in
// a format that the package knows. Match it with errors.Is. Decode also
// returns the errors of the ip, lrrp and nmea packages.
var ErrNoFix = errors.New("location: packet carries no position")
