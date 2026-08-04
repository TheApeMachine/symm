package resonance

/*
errorCalibratorWindow is how many recent errors the calibrator scores against.

The window has to span enough ticks for the empirical distribution to have
resolution finer than the decisions read off it, and few enough that it still
describes the current regime rather than an average of every regime the session
has passed through. Two hundred and fifty six gives a quantile resolution of
about four tenths of a percent, which is finer than any downstream consumer
distinguishes, while turning over fast enough that a regime change washes out of
the window within a few minutes of live ticks.
*/
const errorCalibratorWindow = 256

/*
errorCalibrator reports where a reading falls within the distribution of recent
readings, as the fraction of that distribution the reading beats.

This is deliberately an empirical quantile rather than a parametric transform of
the reading. A prediction error has no fixed scale: it grows with the number of
features in the schema, with market volatility, and with how far the network is
from convergence. Any closed-form map from an error to a confidence therefore
has to assume a scale, and is wrong by however much the live scale differs from
the assumed one. Ranking a reading against its own recent history assumes
nothing, and yields a number that is a probability by construction: it is
uniform on the unit interval whenever the readings are drawn from a stable
distribution, whatever that distribution happens to be.

The retained window is a ring, so the calibrator tracks the current regime and
forgets the ones before it.
*/
type errorCalibrator struct {
	samples []float64
	next    int
	filled  bool
}

/*
newErrorCalibrator returns a calibrator over the retained window.
*/
func newErrorCalibrator() *errorCalibrator {
	return &errorCalibrator{
		samples: make([]float64, errorCalibratorWindow),
	}
}

/*
Quantile folds one reading into the retained window and returns the fraction of
that window the reading is better than, where better means smaller.

The reading is scored against the window as it stood before the reading was
added, so a reading cannot inflate its own rank. The first reading of a session
has nothing to rank against and scores zero, which is the honest answer: no
evidence has been accumulated yet, so no confidence is warranted.
*/
func (calibrator *errorCalibrator) Quantile(reading float64) float64 {
	count := calibrator.count()
	beaten := 0

	for index := range count {
		if reading < calibrator.samples[index] {
			beaten++
		}
	}

	calibrator.samples[calibrator.next] = reading
	calibrator.next = (calibrator.next + 1) % len(calibrator.samples)

	if calibrator.next == 0 {
		calibrator.filled = true
	}

	if count == 0 {
		return 0
	}

	return float64(beaten) / float64(count)
}

/*
count is how many readings the retained window currently holds.
*/
func (calibrator *errorCalibrator) count() int {
	if calibrator.filled {
		return len(calibrator.samples)
	}

	return calibrator.next
}
