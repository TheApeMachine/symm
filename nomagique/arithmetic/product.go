package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Product is the multiplication of a pair that arrived as one value.

Multiply accumulates a run into a register, which is the right operation when
the numbers arrive one after another. A pair is not a run: both numbers are
already in hand, and what is wanted is their product rather than a running
one. That is what lets a pairing operation hand over the operands it found
and leave the multiplying to arithmetic.
*/
type Product struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewProduct configures the value held before anything has been shown.
*/
func NewProduct(state core.Primitive) *Product {
	return &Product{
		current: state,
	}
}

/*
Next multiplies every pair the incoming Primitive yields and holds the last
of the products.
*/
func (product *Product) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([2]float64{}), in, func(_, pair [2]float64) [2]float64 {
			product.current = core.From(pair[0] * pair[1])

			return pair
		},
	)

	product.current.Error(gathered.Error())

	return product.current
}

/*
Read surfaces the product for the boundary.
*/
func (product *Product) Read() any {
	return product.current.Read()
}
