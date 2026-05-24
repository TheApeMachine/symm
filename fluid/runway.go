package fluid

import (
	"math"
	"time"
)

/*
fieldRunway estimates how long the current velocity takes to traverse one spread width.
*/
func fieldRunway(spreadBPS, velocity, elapsedSec float64) time.Duration {
	if elapsedSec <= 0 {
		return 0
	}

	speed := math.Abs(velocity)

	if speed <= 0 || spreadBPS <= 0 {
		return time.Duration(elapsedSec * float64(time.Second))
	}

	seconds := (spreadBPS / 10000) / speed

	if seconds <= 0 {
		return 0
	}

	return time.Duration(seconds * float64(time.Second))
}
