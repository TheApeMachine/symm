/*
Package decimal is shopspring arithmetic with the pointer API the desk already uses.
SDK book I/O still speaks krakenfx decimal; convert only at that boundary.
*/
package decimal

import (
	shop "github.com/shopspring/decimal"
)

const DefaultScale = 12

type Decimal shop.Decimal

func NewFromString(value string) (*Decimal, error) {
	parsed, err := shop.NewFromString(value)
	if err != nil {
		return nil, err
	}

	return wrap(parsed), nil
}

func NewFromFloat64(value float64) *Decimal {
	return wrap(shop.NewFromFloat(value))
}

func NewFromInt64(value int64) *Decimal {
	return wrap(shop.NewFromInt(value))
}

func (d *Decimal) shop() shop.Decimal {
	if d == nil {
		return shop.Zero
	}

	return shop.Decimal(*d)
}

func wrap(value shop.Decimal) *Decimal {
	parsed := Decimal(value)
	return &parsed
}

func (d *Decimal) Copy() *Decimal {
	if d == nil {
		return nil
	}

	return wrap(d.shop())
}

func (d *Decimal) String() string {
	if d == nil {
		return "0"
	}

	return d.shop().String()
}

func (d *Decimal) Float64() (float64, bool) {
	if d == nil {
		return 0, false
	}

	return d.shop().Float64()
}

func (d *Decimal) MustFloat64() float64 {
	value, _ := d.Float64()
	return value
}

func (d *Decimal) Sign() int {
	if d == nil {
		return 0
	}

	return d.shop().Sign()
}

func (d *Decimal) Cmp(other *Decimal) int {
	return d.shop().Cmp(other.shop())
}

func (d *Decimal) Add(other *Decimal) *Decimal {
	return wrap(d.shop().Add(other.shop()))
}

func (d *Decimal) Sub(other *Decimal) *Decimal {
	return wrap(d.shop().Sub(other.shop()))
}

func (d *Decimal) Mul(other *Decimal) *Decimal {
	return wrap(d.shop().Mul(other.shop()))
}

func (d *Decimal) Div(other *Decimal) *Decimal {
	return wrap(d.shop().Div(other.shop()))
}

func (d *Decimal) Abs() *Decimal {
	return wrap(d.shop().Abs())
}

/*
SetScale truncates to scale decimals without rounding.
*/
func (d *Decimal) SetScale(scale int64) *Decimal {
	if d == nil {
		return nil
	}

	return wrap(d.shop().Truncate(int32(scale)))
}

/*
OffsetPercent returns d * (1 + other).
*/
func (d *Decimal) OffsetPercent(other *Decimal) *Decimal {
	return d.Mul(NewFromInt64(1).Add(other))
}

/*
GetSmallestIncrement is one unit at d's current fractional scale.
*/
func (d *Decimal) GetSmallestIncrement() *Decimal {
	places := int32(DefaultScale)

	if d != nil {
		places = -d.shop().Exponent()

		if places < 0 {
			places = 0
		}
	}

	return wrap(shop.New(1, -places))
}

func (d Decimal) MarshalJSON() ([]byte, error) {
	return shop.Decimal(d).MarshalJSON()
}

func (d *Decimal) UnmarshalJSON(data []byte) error {
	var parsed shop.Decimal

	if err := parsed.UnmarshalJSON(data); err != nil {
		return err
	}

	*d = Decimal(parsed)
	return nil
}
