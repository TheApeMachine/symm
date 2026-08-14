package types

type Unit uint8

const (
	UnitUnknown Unit = iota
	UnitDimensionless
	UnitCount
	UnitRate
	UnitDuration
	UnitPrice
	UnitPercent
)

func (unit Unit) String() string {
	switch unit {
	case UnitDimensionless:
		return "dimensionless"
	case UnitCount:
		return "count"
	case UnitRate:
		return "rate"
	case UnitDuration:
		return "duration"
	case UnitPrice:
		return "price"
	case UnitPercent:
		return "percent"
	default:
		return "unknown"
	}
}
