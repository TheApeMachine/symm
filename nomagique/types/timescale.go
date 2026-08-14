package types

type Timescale uint8

const (
	TimescaleUnknown Timescale = iota
	TimescaleInstantaneous
	TimescalePerEpoch
	TimescalePerTick
	TimescalePerNanosecond
	TimescalePerMicrosecond
	TimescalePerMillisecond
	TimescalePerSecond
	TimescalePerMinute
	TimescalePerHour
	TimescalePerDay
	TimescalePerWeek
	TimescalePerMonth
	TimescalePerYear
)

func (timescale Timescale) String() string {
	switch timescale {
	case TimescaleInstantaneous:
		return "instantaneous"
	case TimescalePerEpoch:
		return "per epoch"
	case TimescalePerTick:
		return "per tick"
	case TimescalePerNanosecond:
		return "per nanosecond"
	case TimescalePerMicrosecond:
		return "per microsecond"
	case TimescalePerMillisecond:
		return "per millisecond"
	case TimescalePerSecond:
		return "per second"
	case TimescalePerMinute:
		return "per minute"
	case TimescalePerHour:
		return "per hour"
	case TimescalePerDay:
		return "per day"
	case TimescalePerWeek:
		return "per week"
	case TimescalePerMonth:
		return "per month"
	case TimescalePerYear:
		return "per year"
	default:
		return "unknown"
	}
}
