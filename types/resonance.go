package types

/*
ResonanceMeasurement holds pre-formatted readings and mark price extracted at ingestion time.
*/
type ResonanceMeasurement struct {
	Tick     int64
	Mark     float64
	Readings map[string]float64 // "source:symbol:metric_key" -> normalized value
}

func MeasurementToResonance(symbolName string, measurement *Measurement) *ResonanceMeasurement {
	if measurement == nil {
		return nil
	}

	// 1. Extract Mark Price
	mark := 0.0

	if bid, hasBid := measurement.Metadata["bid"]; hasBid {
		if ask, hasAsk := measurement.Metadata["ask"]; hasAsk && bid > 0 && ask > 0 {
			mark = (bid + ask) / 2
		}
	}

	if mark == 0 {
		if val, found := measurement.Metadata["last_price"]; found && val > 0 {
			mark = val
		}
	}

	if mark == 0 {
		if val, found := measurement.Metadata["trade_price"]; found && val > 0 {
			mark = val
		}
	}

	// 2. Extract Normalized Metrics with Pre-Formatted Keys
	var readings map[string]float64

	if len(measurement.Metrics) > 0 {
		readings = make(map[string]float64, len(measurement.Metrics))

		for key, sample := range measurement.Metrics {
			if sample.Normalized == nil {
				continue
			}

			identity := string(measurement.Source) + ":" + symbolName + ":" + key
			readings[identity] = *sample.Normalized
		}
	}

	// Ignore measurements that carry no usable mark or normalized metrics
	if len(readings) == 0 && mark == 0 {
		return nil
	}

	return &ResonanceMeasurement{
		Tick:     measurement.Tick,
		Mark:     mark,
		Readings: readings,
	}
}
