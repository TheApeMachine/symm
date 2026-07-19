package leadlag

import "math"

/*
windowsFromCount resolves short/long/return-lag counts from sample depth alone.
It matches nomagique ResolveWindows on a zero-filled series of length n, which
is what leadlag used to allocate on every call — without the slice.
*/
func windowsFromCount(sampleCount int) (shortWindow, longWindow, returnLag int) {
	if sampleCount <= 0 {
		return 0, 0, 0
	}

	shortWindow = max(1, int(math.Ceil(math.Sqrt(float64(sampleCount)))))
	longWindow = int(math.Ceil(float64(shortWindow) * 2.0))

	if longWindow <= shortWindow {
		longWindow = shortWindow + 1
	}

	if longWindow > sampleCount {
		longWindow = sampleCount
	}

	returnLag = max(1, int(math.Ceil(math.Sqrt(float64(longWindow)))))

	if longWindow > 1 {
		returnLag = min(returnLag, longWindow-1)
	}

	return shortWindow, longWindow, returnLag
}
