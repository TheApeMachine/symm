package tests

type Symbol struct {
	pair       string
	startPrice float64
	seed       int64
	generator  *Generator
}

func NewSymbol(
	pair string,
	startPrice float64,
	seed int64,
) *Symbol {
	return &Symbol{
		pair:       pair,
		startPrice: startPrice,
		seed:       seed,
		generator:  NewGenerator(pair, startPrice, seed),
	}
}
