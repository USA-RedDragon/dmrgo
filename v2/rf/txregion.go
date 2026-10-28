package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

// Region is the regulatory region whose limits a transmitter keeps. The
// zero value RegionWorld keeps the limits of every region at once.
type Region uint8

// The regions.
const (
	// RegionWorld keeps the limits of every other region at once, the
	// strictest of each.
	RegionWorld Region = iota
	// RegionUS keeps 47 CFR 90.210 Emission Mask D for land mobile and 47
	// CFR 97.307 for amateur. It does not cover the 800 MHz bands of Mask
	// H or the 90.691 EA mask.
	RegionUS
	// RegionCEPT keeps ETSI EN 300 113 for land mobile and ERC/REC 74-01
	// for amateur.
	RegionCEPT
	// RegionChina keeps the land mobile limits of the 2025 MIIT notice.
	RegionChina
	// RegionJapan keeps the land mobile limits of the Radio Equipment
	// Regulations.
	RegionJapan
)

// String returns the name of the region. It allocates only for a value
// without a name.
func (r Region) String() string {
	switch r {
	case RegionWorld:
		return "World"
	case RegionUS:
		return "US"
	case RegionCEPT:
		return "CEPT"
	case RegionChina:
		return "China"
	case RegionJapan:
		return "Japan"
	}
	return "Region(" + itoa(int(r)) + ")"
}

// RegionError is the error of a transmitter setup that the Region,
// Service and Power do not allow, or that no measurement shows to keep
// their limits. Each such error has one static RegionError, so a call that
// fails does not allocate. Match it with errors.As.
type RegionError struct {
	// Type is the package, "rf", and Field the config field that the
	// limits do not allow, such as "TxConfig.Rate".
	Type, Field string
	// Reason gives the cause.
	Reason string
}

// Error returns the message of e. It allocates the message.
func (e *RegionError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Type + ": " + e.Field + " is not allowed: " + e.Reason
}

const (
	regions        = int(RegionJapan) + 1
	licenseFreeDBm = 27
	modeledDBm     = 60
	maxPowerDBm    = 63
)

var ceilingDBm = [regions][2]int8{
	RegionWorld: {60, 62},
	RegionUS:    {60, 62},
	RegionCEPT:  {60, 62},
	RegionChina: {44, 44},
	RegionJapan: {60, 60},
}

var spurGate10 [regions][2]bool

type txErrs struct {
	tier, rate, format, offset, power, bits, level, offsetCodes, onSamples, lowpass, preEmphSet *dmrgo.ConfigError
	rateLimit, power60, powerCeil, service, noLF, amateur, spur, preEmph, levelLow, region, svc *RegionError
}

var iqErrs = txErrs{
	tier:        &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Tier"},
	rate:        &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Rate"},
	format:      &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Format"},
	offset:      &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.OffsetHz"},
	power:       &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Power"},
	bits:        &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Bits"},
	level:       &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Level"},
	offsetCodes: &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.Offset"},
	onSamples:   &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.OnSamples"},
	lowpass:     &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.InputLowpassHz"},
	preEmphSet:  &dmrgo.ConfigError{Type: "rf", Field: "TxConfig.PreEmphasis"},
	rateLimit:   &RegionError{Type: "rf", Field: "TxConfig.Rate", Reason: "no measurement keeps the limits at this rate"},
	power60:     &RegionError{Type: "rf", Field: "TxConfig.Power", Reason: "the limits above 60 dBm are not measured"},
	powerCeil:   &RegionError{Type: "rf", Field: "TxConfig.Power", Reason: "above the highest legal power of the service in the region"},
	service:     &RegionError{Type: "rf", Field: "TxConfig.Service", Reason: "the region has no license-free DMR service"},
	noLF:        &RegionError{Type: "rf", Field: "TxConfig.Service", Reason: "the limits of the license-free service are not measured"},
	amateur:     &RegionError{Type: "rf", Field: "TxConfig.Service", Reason: "the amateur limits of the region are not measured"},
	spur:        &RegionError{Type: "rf", Field: "TxConfig.Region", Reason: "the signal is not measured 10 dB under the spurious domain limits"},
	preEmph:     &RegionError{Type: "rf", Field: "TxConfig.PreEmphasis", Reason: "the limits with pre-emphasis are not measured"},
	levelLow:    &RegionError{Type: "rf", Field: "TxConfig.Level", Reason: "under the level that the limits are measured at for Bits"},
	region:      &RegionError{Type: "rf", Field: "TxConfig.Region", Reason: "unknown region"},
	svc:         &RegionError{Type: "rf", Field: "TxConfig.Service", Reason: "unknown service"},
}

var audioErrs = txErrs{
	tier:        &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Tier"},
	rate:        &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Rate"},
	format:      &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Format"},
	offset:      &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.OffsetHz"},
	power:       &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Power"},
	bits:        &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Bits"},
	level:       &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Level"},
	offsetCodes: &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.Offset"},
	onSamples:   &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.OnSamples"},
	lowpass:     &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.InputLowpassHz"},
	preEmphSet:  &dmrgo.ConfigError{Type: "rf", Field: "AudioTxConfig.PreEmphasis"},
	rateLimit:   &RegionError{Type: "rf", Field: "AudioTxConfig.Rate", Reason: "no measurement keeps the limits at this rate"},
	power60:     &RegionError{Type: "rf", Field: "AudioTxConfig.Power", Reason: "the limits above 60 dBm are not measured"},
	powerCeil:   &RegionError{Type: "rf", Field: "AudioTxConfig.Power", Reason: "above the highest legal power of the service in the region"},
	service:     &RegionError{Type: "rf", Field: "AudioTxConfig.Service", Reason: "the region has no license-free DMR service"},
	noLF:        &RegionError{Type: "rf", Field: "AudioTxConfig.Service", Reason: "the limits of the license-free service are not measured"},
	amateur:     &RegionError{Type: "rf", Field: "AudioTxConfig.Service", Reason: "the amateur limits of the region are not measured"},
	spur:        &RegionError{Type: "rf", Field: "AudioTxConfig.Region", Reason: "the signal is not measured 10 dB under the spurious domain limits"},
	preEmph:     &RegionError{Type: "rf", Field: "AudioTxConfig.PreEmphasis", Reason: "with pre-emphasis only Tier0 at 48000 Hz keeps the limits, and not in every region, service and power"},
	levelLow:    &RegionError{Type: "rf", Field: "AudioTxConfig.Level", Reason: "under the level that the limits are measured at for Bits"},
	region:      &RegionError{Type: "rf", Field: "AudioTxConfig.Region", Reason: "unknown region"},
	svc:         &RegionError{Type: "rf", Field: "AudioTxConfig.Service", Reason: "unknown service"},
}

type txRegs struct {
	region  Region
	service Service
	power   int
}

type txGate struct{ noSpur bool }

func (r *txRegs) check(e *txErrs, g *txGate) (class int, err error) {
	if r.region >= Region(regions) {
		return 0, e.region
	}
	if r.power < 0 || r.power > maxPowerDBm {
		return 0, e.power
	}
	var top int
	switch r.service {
	case ServiceLicenseFree:
		if r.region == RegionUS || r.region == RegionJapan {
			return 0, e.service
		}
		top = licenseFreeDBm
	case ServiceUnknown:
		top = int(max(ceilingDBm[r.region][0], ceilingDBm[r.region][1]))
	case ServiceLandMobile:
		top = int(ceilingDBm[r.region][0])
	case ServiceAmateur:
		top = int(ceilingDBm[r.region][1])
	default:
		return 0, e.svc
	}
	p := r.power
	if p == 0 {
		p = top
	}
	if p > top {
		return 0, e.powerCeil
	}
	if r.service == ServiceLicenseFree {
		return 0, e.noLF
	}
	if p > modeledDBm {
		return 0, e.power60
	}
	if r.service != ServiceLandMobile && (r.region == RegionChina || r.region == RegionJapan) {
		return 0, e.amateur
	}
	if !g.noSpur && !r.spurMeasured() {
		return 0, e.spur
	}
	switch {
	case p <= 30:
		return fsktx.Limits1W, nil
	case p <= 40:
		return fsktx.Limits10W, nil
	case p <= 50:
		return fsktx.Limits100W, nil
	}
	return fsktx.Limits1000W, nil
}

func (r *txRegs) spurMeasured() bool {
	for _, s := range r.services() {
		if s < 0 {
			continue
		}
		for _, g := range r.regionsOf() {
			if g >= 0 && !spurGate10[g][s] {
				return false
			}
		}
	}
	return true
}

func (r *txRegs) regionsOf() [regions]int {
	if r.region != RegionWorld {
		return [regions]int{int(r.region), -1, -1, -1, -1}
	}
	return [regions]int{1, 2, 3, 4, -1}
}

func (r *txRegs) services() [2]int {
	switch r.service {
	case ServiceLandMobile:
		return [2]int{fsktx.LimitsLandMobile, -1}
	case ServiceAmateur:
		return [2]int{fsktx.LimitsAmateur, -1}
	case ServiceUnknown, ServiceLicenseFree:
	}
	return [2]int{fsktx.LimitsLandMobile, fsktx.LimitsAmateur}
}

func (r *txRegs) minRate(t Tier, iq bool, bits, class, lowpass int, pe bool) int {
	least := 0
	for _, s := range r.services() {
		if s < 0 {
			continue
		}
		m := fsktx.Setup{Tier: fsktx.Tier(t), IQ: iq, Bits: bits, Region: int(r.region), Service: s, Power: class, InputLowpassHz: lowpass, PreEmphasis: pe}.MinRate()
		if m == 0 {
			return 0
		}
		least = max(least, m)
	}
	return least
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Service is the radio service of a transmitting station. It is
// dmrgo.Service, the type that package privacy also takes.
type Service = dmrgo.Service

// The Service values.
const (
	// ServiceUnknown is a service that the caller did not state: the
	// transmitter keeps the limits of the land mobile and amateur services
	// at once.
	ServiceUnknown = dmrgo.ServiceUnknown
	// ServiceAmateur is the amateur or amateur-satellite service.
	ServiceAmateur = dmrgo.ServiceAmateur
	// ServiceLandMobile is a licensed land mobile service, such as private,
	// business or public safety PMR.
	ServiceLandMobile = dmrgo.ServiceLandMobile
	// ServiceLicenseFree is a licence-exempt service, such as PMR446.
	ServiceLicenseFree = dmrgo.ServiceLicenseFree
)
