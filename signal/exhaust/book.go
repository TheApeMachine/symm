package exhaust

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Book struct {
	engine *Engine
}

func NewBook(engine *Engine) *Book {
	return &Book{
		engine: engine,
	}
}

func (book *Book) Measure(row kraken.BookData) (*logic.Measurement, error) {
	return book.engine.MeasureBook(row)
}
