package utils

import (
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
)

func Add(
	out datura.Map[datura.Map[*decimal.Decimal]],
	value *decimal.Decimal,
	keys ...string,
) datura.Map[datura.Map[*decimal.Decimal]] {
	out[keys[0]][keys[1]] = out[keys[0]][keys[1]].Add(value)
	return out
}
