package causal

import (
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	pearl *algorithm.Pearl
}

func NewBook() *Book {
	return &Book{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{}),
	}
}

func (book *Book) Measure(
	row kraken.BookData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	output, ready, err := book.pearl.MeasureBook(algorithm.PearlBookInput{
		Symbol: row.Symbol,
		Bids:   row.Bids,
		Asks:   row.Asks,
	})
	if err != nil || !ready {
		return nil, err
	}

	return []*types.Measurement{&types.Measurement{}}, nil
}
