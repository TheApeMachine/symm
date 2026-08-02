package types

type Symbol struct {
	Pair       string
	StartPrice float64
	Seed       int64
}

func NewSymbol(
	pair string,
	startPrice float64,
	seed int64,
) *Symbol {
	return &Symbol{
		Pair:       pair,
		StartPrice: startPrice,
		Seed:       seed,
	}
}
