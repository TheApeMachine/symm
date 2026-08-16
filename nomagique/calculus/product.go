package calculus

import "github.com/theapemachine/symm/nomagique/types"

/*
Product multiplies two finite scalar operands carried as "left" and "right".
*/
type Product struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*Product)(nil)

func NewProduct(initial types.Input[scalarMap]) *Product {
	return &Product{initial: initial, next: types.NewInput[scalarMap]()}
}

func (product *Product) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "product")
	if err != nil {
		product.next = types.NewErrorInput(mapping, err)
		return
	}
	product.next = scalarInput(mapping)
}

func (product *Product) Read() types.IO[scalarMap] {
	if product.next.Error() != "" {
		return product.next
	}

	mapping := product.next.Project().Read()
	left, hasLeft := scalar(mapping, "left")
	right, hasRight := scalar(mapping, "right")
	if !hasLeft || !hasRight {
		product.next = types.NewErrorInput(mapping,
			scalarValidation("product", "missing left or right"))
		return product.next
	}
	if !finite(left, right) {
		product.next = types.NewErrorInput(mapping,
			scalarValidation("product", "operands must be finite"))
		return product.next
	}

	putScalar(mapping, "result", left*right)
	product.initial = scalarInput(mapping)
	product.next = scalarInput(mapping)
	return product.next
}

func (product *Product) Project() types.Value[scalarMap] { return product.next.Project() }
func (product *Product) Error() string                   { return product.next.Error() }
func (product *Product) Close() error {
	if product.initial != nil {
		if err := product.initial.Close(); err != nil {
			return err
		}
	}
	if product.next != nil {
		if err := product.next.Close(); err != nil {
			return err
		}
	}
	product.next = types.NewInput[scalarMap]()
	return nil
}
