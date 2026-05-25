package numeric

/*
PhaseDialPrimeCount is the number of prime frequencies used for PhaseDial
and PhaseRotor encodings (one prime ω per complex / rotor dimension).
*/
const PhaseDialPrimeCount = 512

/*
PhaseDialPrimes holds the first PhaseDialPrimeCount primes, filled at init.
Dimension k uses ω = PhaseDialPrimes[k] as the phase accumulation rate.
*/
var PhaseDialPrimes [PhaseDialPrimeCount]uint64

func init() {
	const sieveMax = 10000

	sieve := make([]bool, sieveMax+1)

	for idx := 2; idx <= sieveMax; idx++ {
		sieve[idx] = true
	}

	for prime := 2; prime*prime <= sieveMax; prime++ {
		if !sieve[prime] {
			continue
		}

		for multiple := prime * prime; multiple <= sieveMax; multiple += prime {
			sieve[multiple] = false
		}
	}

	count := 0

	for candidate := 2; candidate <= sieveMax && count < PhaseDialPrimeCount; candidate++ {
		if !sieve[candidate] {
			continue
		}

		PhaseDialPrimes[count] = uint64(candidate)
		count++
	}

	if count < PhaseDialPrimeCount {
		panic("numeric: sieve did not yield enough primes for PhaseDial")
	}
}
