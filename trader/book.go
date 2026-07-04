package trader

import (
	"time"

	"github.com/theapemachine/symm/kraken"
)

type Book struct {
	history *History[kraken.BookData]
}

func NewBook() *Book {
	return &Book{
		history: NewHistory[kraken.BookData](),
	}
}

func (book *Book) Measure(message kraken.BookDataSlice) (time.Time, error) {
	var latest time.Time

	for _, msg := range message {
		if err := book.history.Measure(msg.Symbol, msg.Timestamp, msg); err != nil {
			return time.Time{}, err
		}

		if msg.Timestamp.After(latest) {
			latest = msg.Timestamp
		}
	}

	return latest, nil
}
