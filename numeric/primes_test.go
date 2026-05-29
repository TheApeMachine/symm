package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPhaseDialPrimes(t *testing.T) {
	Convey("Given the phase-dial prime table", t, func() {
		Convey("It should fill the requested prime count", func() {
			So(PhaseDialPrimes[0], ShouldEqual, 2)
			So(PhaseDialPrimes[PhaseDialPrimeCount-1], ShouldBeGreaterThan, 0)
		})

		Convey("It should store primes in ascending order", func() {
			for index := 1; index < PhaseDialPrimeCount; index++ {
				So(PhaseDialPrimes[index], ShouldBeGreaterThan, PhaseDialPrimes[index-1])
				So(isPrime(PhaseDialPrimes[index]), ShouldBeTrue)
			}
		})
	})
}

func isPrime(candidate uint64) bool {
	if candidate < 2 {
		return false
	}

	if candidate == 2 {
		return true
	}

	if candidate%2 == 0 {
		return false
	}

	for divisor := uint64(3); divisor*divisor <= candidate; divisor += 2 {
		if candidate%divisor == 0 {
			return false
		}
	}

	return true
}

func BenchmarkPhaseDialPrimesAccess(b *testing.B) {
	index := 0

	for b.Loop() {
		_ = PhaseDialPrimes[index%PhaseDialPrimeCount]
		index++
	}
}
