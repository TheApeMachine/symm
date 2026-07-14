package trader

import (
	"fmt"
	"iter"

	orderbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
)

/*
SDKLevel3Book validates authoritative L3 rows against the SDK BookManager
that already applied the same websocket frames.
*/
type SDKLevel3Book struct {
	managers iter.Seq[*spot.BookManager]
	invalid  map[string]manifold.InvalidReason
}

/*
NewSDKLevel3Book wraps the transport book managers used for level3 checksums.
*/
func NewSDKLevel3Book(
	managers iter.Seq[*spot.BookManager],
) *SDKLevel3Book {
	return &SDKLevel3Book{
		managers: managers,
		invalid:  map[string]manifold.InvalidReason{},
	}
}

/*
ForSymbol returns one symbol-local book view for manifold observation.
*/
func (book *SDKLevel3Book) ForSymbol(symbol string) manifold.Level3Book {
	return &sdkLevel3SymbolBook{
		parent: book,
		symbol: symbol,
	}
}

func (sdkBook *SDKLevel3Book) bookForSymbol(symbol string) *orderbook.Book {
	for manager := range sdkBook.managers {
		if manager == nil {
			continue
		}

		instance := manager.GetBook(symbol)

		if instance != nil {
			return instance
		}
	}

	return nil
}

type sdkLevel3SymbolBook struct {
	parent *SDKLevel3Book
	symbol string
}

func (book *sdkLevel3SymbolBook) Apply(
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
) bool {
	_ = pricePrecision
	_ = qtyPrecision

	if row.Symbol != book.symbol {
		book.parent.invalid[book.symbol] = manifold.BookInvalid
		return false
	}

	instance := book.parent.bookForSymbol(book.symbol)

	if instance == nil {
		book.parent.invalid[book.symbol] = manifold.BookInvalid
		return false
	}

	if row.Checksum == 0 {
		bestBid, bestAsk := instance.BestBid(), instance.BestAsk()

		if bestBid == nil || bestAsk == nil {
			book.parent.invalid[book.symbol] = manifold.BookInvalid
			return false
		}

		book.parent.invalid[book.symbol] = manifold.Valid
		return true
	}

	result := instance.L3Checksum(fmt.Sprint(row.Checksum))

	if !result.Match {
		book.parent.invalid[book.symbol] = manifold.ChecksumFailed
		return false
	}

	book.parent.invalid[book.symbol] = manifold.Valid
	return true
}

func (book *sdkLevel3SymbolBook) TopOfBook(
	symbol string,
) (float64, float64, bool) {
	if symbol != book.symbol {
		return 0, 0, false
	}

	instance := book.parent.bookForSymbol(book.symbol)

	if instance == nil {
		return 0, 0, false
	}

	bestBid, bestAsk := instance.BestBid(), instance.BestAsk()

	if bestBid == nil || bestAsk == nil {
		return 0, 0, false
	}

	bid := bestBid.Price.Float64()
	ask := bestAsk.Price.Float64()

	if bid <= 0 || ask <= 0 {
		return 0, 0, false
	}

	return bid, ask, true
}

func (book *sdkLevel3SymbolBook) InvalidReason(
	symbol string,
) manifold.InvalidReason {
	if symbol != book.symbol {
		return manifold.BookInvalid
	}

	reason := book.parent.invalid[book.symbol]

	if reason == "" {
		return manifold.Valid
	}

	return reason
}
