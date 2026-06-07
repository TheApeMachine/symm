package reasoning

type UnitType uint8

const (
	UnitNone UnitType = iota
	UnitPercentage
	UnitPips
	UnitPoints
	UnitTicks
	UnitTimeYears
	UnitTimeMonths
	UnitTimeWeeks
	UnitTimeDays
	UnitTimeHours
	UnitTimeMinutes
	UnitTimeSeconds
	UnitTimeMilliseconds
	UnitTimeMicroseconds
	UnitTimeNanoseconds
	UnitConfidence
	UnitSNR
	// UnitStrength reads the signal's raw fused magnitude (Measurement.Strength) —
	// the physical quantity itself (RVOL lift, depth notional, drift), as opposed
	// to SNR (how SURPRISING the reading is) and Confidence (how BELIEVED the
	// category selection is). Scales are signal-specific: gate per category, or
	// use rose_by to read the magnitude's own momentum scale-free.
	UnitStrength
)
