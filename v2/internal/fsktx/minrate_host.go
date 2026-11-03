//go:build !tinygo

package fsktx

func minRateHost(ti int) (minRateTab, bool) {
	if ti < 1 || ti > len(minRateAudioH) {
		return minRateTab{}, false
	}
	i := ti - 1
	return minRateTab{rows: minRateRowsH[:], audio: &minRateAudioH[i], pe: &minRatePEH[i], iq: &minRateIQH[i], iqRates: minRateIQRatesH[:]}, true
}
