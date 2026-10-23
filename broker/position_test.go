package broker_test

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/broker"
)

/*
Virtual lots exit on fee-net book edge (book_return), not mark over entry.
*/
func TestShouldExitUsesBookReturnThreshold(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())
	position := broker.NewPosition("AAVE/USD", "R01/R02", true, price)

	position.BookReturn = ptrFloat64(0)

	if position.ShouldExit() {
		t.Fatal("flat book edge should stay open")
	}

	position.BookReturn = ptrFloat64(1.99)

	if position.ShouldExit() {
		t.Fatal("1.99% book edge should stay open")
	}

	position.BookReturn = ptrFloat64(2)

	if !position.ShouldExit() {
		t.Fatal("2% book edge should take profit")
	}

	position.BookReturn = ptrFloat64(-1.99)

	if position.ShouldExit() {
		t.Fatal("-1.99% book edge should stay open")
	}

	position.BookReturn = ptrFloat64(-2)

	if !position.ShouldExit() {
		t.Fatal("-2% book edge should stop out")
	}
}

func TestShouldExitWithoutBookReturn(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())
	position := broker.NewPosition("AAVE/USD", "R01/R02", true, price)
	position.EntryPrice = decimal.NewFromFloat64(100)
	position.Mark = decimal.NewFromFloat64(150)

	if position.ShouldExit() {
		t.Fatal("mark move without book_return must not exit")
	}
}

/*
A paper lot leaves the desk on the action tree, not on the book-edge target.
*/
func TestShouldExitIgnoresPaperPositions(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())

	position := broker.NewPosition("AAVE/USD", "R01/R02", false, price)
	position.BookReturn = ptrFloat64(5)

	if position.ShouldExit() {
		t.Fatal("a paper position closed on the virtual book-edge target")
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
