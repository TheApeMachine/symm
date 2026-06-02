package market

/*
gaugeReadings holds the latest per-symbol clarity and SNR for each dashboard
source. Story flushes cross-sectional means on the UI ticker, not lifetime maxima.
*/
type gaugeReadings struct {
	bySource map[string]*gaugeSourceReadings
}

type gaugeSourceReadings struct {
	clarity map[string]float64
	snr     map[string]float64
}

func newGaugeReadings() gaugeReadings {
	return gaugeReadings{
		bySource: make(map[string]*gaugeSourceReadings),
	}
}

func gaugeSymbolKey(symbol string) string {
	if symbol == "" {
		return "_"
	}

	return symbol
}

func (readings *gaugeReadings) record(
	source, symbol string,
	confidence, snr float64,
) {
	entry := readings.bySource[source]

	if entry == nil {
		entry = &gaugeSourceReadings{
			clarity: make(map[string]float64),
			snr:     make(map[string]float64),
		}
		readings.bySource[source] = entry
	}

	key := gaugeSymbolKey(symbol)
	entry.clarity[key] = confidence

	if snr > 0 {
		entry.snr[key] = snr
	}
}

func (readings gaugeReadings) meanClarity(source string) (mean float64, count int) {
	entry := readings.bySource[source]

	if entry == nil || len(entry.clarity) == 0 {
		return 0, 0
	}

	var sum float64

	for _, value := range entry.clarity {
		sum += value
	}

	return sum / float64(len(entry.clarity)), len(entry.clarity)
}

func (readings gaugeReadings) meanSNR(source string) float64 {
	entry := readings.bySource[source]

	if entry == nil || len(entry.snr) == 0 {
		return 0
	}

	var sum float64

	for _, value := range entry.snr {
		sum += value
	}

	return sum / float64(len(entry.snr))
}

func (readings gaugeReadings) sourceSNRMeans() map[string]float64 {
	means := make(map[string]float64, len(readings.bySource))

	for source := range readings.bySource {
		mean := readings.meanSNR(source)

		if mean > 0 {
			means[source] = mean
		}
	}

	return means
}
