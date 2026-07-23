package utils

import (
	"math"
	"time"
)

func Backoff(n int) int {
	n = int(
		math.Round((math.Pow(
			math.Phi, float64(n),
		) + math.Pow(
			math.Phi-1, float64(n),
		)) / math.Sqrt(5)),
	)

	time.Sleep(time.Duration(n) * time.Millisecond)
	return n
}
