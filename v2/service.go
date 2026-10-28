package dmrgo

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Service is the radio service of a transmitting station. Package privacy
// uses it to check whether the station may encrypt, and package rf to
// choose the regulatory limits that its signal keeps. A Service names no
// region.
type Service uint8

// The Service values.
const (
	// ServiceUnknown is a service that the caller did not state.
	ServiceUnknown Service = iota
	// ServiceAmateur is the amateur or amateur-satellite service.
	ServiceAmateur
	// ServiceLandMobile is a licensed land mobile service, such as private,
	// business or public safety PMR.
	ServiceLandMobile
	// ServiceLicenseFree is a licence-exempt service, such as PMR446.
	ServiceLicenseFree
)

// String returns the name of the service. It allocates only for a value
// without a name.
func (s Service) String() string {
	switch s {
	case ServiceUnknown:
		return "Unknown"
	case ServiceAmateur:
		return "Amateur"
	case ServiceLandMobile:
		return "LandMobile"
	case ServiceLicenseFree:
		return "LicenseFree"
	}
	return "Service(" + text.Uint(uint64(s)) + ")"
}
