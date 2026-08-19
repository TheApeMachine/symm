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
	UnitQuoteCurrency
	UnitBaseCurrency
	UnitEventsPerSecond
	UnitInverseSecond
	UnitNat
	UnitSecond
	UnitQuoteCurrencyPerSecond
	UnitBaseCurrencyPerSecond
	UnitInverseQuoteCurrencySecond
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
	case UnitQuoteCurrency:
		return "quote_currency"
	case UnitBaseCurrency:
		return "base_currency"
	case UnitEventsPerSecond:
		return "events_per_second"
	case UnitInverseSecond:
		return "inverse_second"
	case UnitNat:
		return "nat"
	case UnitSecond:
		return "second"
	case UnitQuoteCurrencyPerSecond:
		return "quote_currency_per_second"
	case UnitBaseCurrencyPerSecond:
		return "base_currency_per_second"
	case UnitInverseQuoteCurrencySecond:
		return "inverse_quote_currency_second"
	default:
		return "unknown"
	}
}
