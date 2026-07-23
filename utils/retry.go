package utils

import (
	"math"
	"time"
)

func Backoff(n int) int {
	next := int(math.Round(float64(n) * math.Phi))
	time.Sleep(time.Duration(next) * time.Millisecond)
	return next
}
