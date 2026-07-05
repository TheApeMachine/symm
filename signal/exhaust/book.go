package exhaust

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	engine *Engine
}

func NewBook(engine *Engine) *Book {
	return &Book{
		engine: engine,
	}
}

func (book *Book) Measure(row kraken.BookData) ([]*types.Measurement, error) {
	measurement, err := book.engine.MeasureBook(row)

	if err != nil || measurement == nil {
		return nil, err
	}

	return []*types.Measurement{measurement}, nil
}
