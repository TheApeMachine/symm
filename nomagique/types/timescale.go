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

func ParseTimescale(name string) Timescale {
	switch name {
	case "instantaneous":
		return TimescaleInstantaneous
	case "per epoch", "epoch":
		return TimescalePerEpoch
	case "per tick", "tick":
		return TimescalePerTick
	case "per nanosecond", "nanosecond":
		return TimescalePerNanosecond
	case "per microsecond", "microsecond":
		return TimescalePerMicrosecond
	case "per millisecond", "millisecond":
		return TimescalePerMillisecond
	case "per second", "second":
		return TimescalePerSecond
	case "per minute", "minute":
		return TimescalePerMinute
	case "per hour", "hour":
		return TimescalePerHour
	case "per day", "day":
		return TimescalePerDay
	case "per week", "week":
		return TimescalePerWeek
	case "per month", "month":
		return TimescalePerMonth
	case "per year", "year":
		return TimescalePerYear
	default:
		return TimescaleUnknown
	}
}
