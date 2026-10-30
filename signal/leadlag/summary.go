package leadlag

/*
leadLagReading is one peer's cross-lag result: the lag between the peer
(reference) and the focal (measured) path, the correlation at that lag, and the
gain over the zero-lag baseline.
*/
type leadLagReading struct {
	lagSeconds  float64
	corr        float64
	leads       bool
	gain        float64
	gainDefined bool
}

/*
ledPeerShare is the fraction of benchmark peers the focal symbol temporally
leads (negative best lag in the cross-lag search). ok is false when there are
no eligible peer readings.
*/
func ledPeerShare(readings []leadLagReading) (share float64, ok bool) {
	if len(readings) == 0 {
		return 0, false
	}

	led := 0
	for _, reading := range readings {
		if reading.leads && reading.lagSeconds < 0 {
			led++
		}
	}

	return float64(led) / float64(len(readings)), true
}
