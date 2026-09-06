package learning

/*
ConditionToken preserves a quantity's identity and the directions of its level
relative to its causal baseline and its latest change. Zero, positive and
negative are exact order relations, not selected thresholds. Magnitude and
measurement quality remain in Grid activity and observation authority.

Bit 52 distinguishes conditioned tokens from historical quantity IDs.
Four low bits hold the two ternary signs; the remaining 48 bits name a quantity, keeping tokens exact in JSON/JavaScript.
*/
func ConditionToken(quantity uint64, level, change float64) uint64 {
	if quantity == 0 || quantity >= 1<<48 {
		panic("learning: condition quantity does not fit token encoding")
	}
	state := uint64(0)
	for index, value := range [2]float64{level, change} {
		if value > 0 {
			state |= 1 << (index * 2)
		}
		if value < 0 {
			state |= 2 << (index * 2)
		}
	}
	return 1<<52 | quantity<<4 | state
}

/* RemapCondition replaces only the named quantity when interning historical inputs. */
func RemapCondition(token, quantity uint64) uint64 {
	if token>>52 == 0 {
		return quantity
	}
	if quantity == 0 || quantity >= 1<<48 {
		panic("learning: condition quantity does not fit token encoding")
	}
	return 1<<52 | quantity<<4 | token&15
}
