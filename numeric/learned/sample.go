package learned

const maxSampleRatio = 2.0

/*
SampleRatio maps one predicted outcome and its realized value to a calibration
sample in [0, maxSampleRatio]. Wins scale by actual/predicted; losses preserve
magnitude via 1+actual/predicted clamped at zero.
*/
func SampleRatio(predicted, actual float64) (float64, bool) {
	if predicted <= 0 {
		return 0, false
	}

	ratio := actual / predicted

	if actual <= 0 {
		ratio = 1 + ratio
	}

	if ratio < 0 {
		ratio = 0
	}

	if ratio > maxSampleRatio {
		ratio = maxSampleRatio
	}

	return ratio, true
}
