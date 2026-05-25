package adaptive

/*
Product multiplies out with every observation in values. Any non-positive
operand yields zero.
*/
type Product struct{}

/*
NewProduct creates a multiplicative gate dynamic.
*/
func NewProduct() *Product {
	return &Product{}
}

/*
Next returns the product of out (when positive) and all values.
When out is zero, multiplication starts at one.
*/
func (product *Product) Next(out float64, values ...float64) (float64, error) {
	result := out

	if result <= 0 {
		result = 1
	}

	for _, observation := range values {
		if observation <= 0 {
			return 0, nil
		}

		result *= observation
	}

	return result, nil
}

/*
Reset is a no-op for Product.
*/
func (product *Product) Reset() error {
	return nil
}
