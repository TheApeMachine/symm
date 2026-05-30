package config

import "time"

/*
BarInterval returns the grid step used for synchronized return sampling,
derived from the correlation-bar (or candle) seconds in system config. It lives
here, not in numeric, so the numeric layer stays free of application config.
*/
func BarInterval() time.Duration {
	seconds := System.CorrelationBarSeconds

	if seconds <= 0 {
		seconds = System.CandleSeconds
	}

	if seconds <= 0 {
		return time.Second
	}

	return time.Duration(seconds) * time.Second
}
