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

func ParseUnit(name string) Unit {
	switch name {
	case "dimensionless":
		return UnitDimensionless
	case "count":
		return UnitCount
	case "rate":
		return UnitRate
	case "duration":
		return UnitDuration
	case "price":
		return UnitPrice
	case "percent":
		return UnitPercent
	case "quote_currency":
		return UnitQuoteCurrency
	case "base_currency":
		return UnitBaseCurrency
	case "events_per_second":
		return UnitEventsPerSecond
	case "per_second":
		// The data-layer spelling of an arrival/rate unit; events per second
		// is the natural reading for arrival rates.
		return UnitEventsPerSecond
	case "inverse_second":
		return UnitInverseSecond
	case "nat":
		return UnitNat
	case "second":
		return UnitSecond
	case "quote_currency_per_second":
		return UnitQuoteCurrencyPerSecond
	case "base_currency_per_second":
		return UnitBaseCurrencyPerSecond
	case "inverse_quote_currency_second":
		return UnitInverseQuoteCurrencySecond
	default:
		return UnitUnknown
	}
}
