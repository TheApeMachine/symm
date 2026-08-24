package data

/*
Unit describes the physical dimension of a measured value. It is a plain
string-converted type so domain layers can register their own units without
forcing nomagique to enumerate market currencies. The generic units below are
the universally meaningful ones only.
*/
type Unit string

const (
	UnitDimensionless Unit = "dimensionless"
	UnitCount         Unit = "count"
	UnitRate          Unit = "rate"
	UnitDuration      Unit = "duration"
	UnitPercent       Unit = "percent"
	UnitSecond        Unit = "second"
	UnitPerSecond     Unit = "per_second"
	UnitNat           Unit = "nat"
)

/*
Timescale describes the period over which a measured value accrues. Only the
generic reciprocal-of-time and point scales live here; market session notions
such as ticks or epochs belong to the domain layer.
*/
type Timescale string

const (
	TimescaleInstantaneous Timescale = "instantaneous"
	TimescalePerSecond     Timescale = "per_second"
	TimescalePerMinute     Timescale = "per_minute"
	TimescalePerHour       Timescale = "per_hour"
	TimescalePerDay        Timescale = "per_day"
)
