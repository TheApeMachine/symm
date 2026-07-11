package logic

import (
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"

	. "github.com/smartystreets/goconvey/convey"
)

type testLevel3Book struct {
	bid float64
	ask float64
}

func (book testLevel3Book) Apply(kraken.Level3Data, int, int) bool { return true }

func (book testLevel3Book) Invalid(string) bool { return false }

func (book testLevel3Book) TopOfBook(string) (float64, float64, bool) {
	if book.bid <= 0 || book.ask <= 0 {
		return 0, 0, false
	}

	return book.bid, book.ask, true
}

func init() {
	viper.Set("market.l3_depth", 10)
	viper.Set("trading.edge.forward_return_horizon", 5*time.Minute)
}

func TestAnalyzerRejectsEmptySymbol(t *testing.T) {
	Convey("Given level3 rows without a symbol", t, func() {
		analyzer := NewAnalyzer(nil, nil)
		book := testLevel3Book{bid: 99, ask: 101}

		Convey("When the analyzer ingests the row", func() {
			theses := analyzer.IngestLevel3(kraken.Level3Data{}, 1, 8, book)

			Convey("Then no thesis is produced", func() {
				So(theses, ShouldBeNil)
			})
		})
	})
}

func TestAnalyzerIngestLevel3(t *testing.T) {
	Convey("Given an analyzer and valid level3 snapshot row", t, func() {
		analyzer := NewAnalyzer(nil, nil)
		book := testLevel3Book{bid: 99, ask: 101}
		row := kraken.Level3Data{
			Symbol:    "BTC/USD",
			Type:      "snapshot",
			Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2,
				Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(1, 0),
			}},
		}

		theses := analyzer.IngestLevel3(row, 1, 8, book)

		Convey("It should admit a field engine slot for the symbol", func() {
			So(theses, ShouldHaveLength, 1)
			_, ok := analyzer.engine.Slot("BTC/USD")
			So(ok, ShouldBeTrue)
		})
	})
}

func BenchmarkAnalyzerIngestLevel3ColdSymbols(b *testing.B) {
	analyzer := NewAnalyzer(nil, nil)
	book := testLevel3Book{}

	for index := 0; index < b.N; index++ {
		analyzer.IngestLevel3(kraken.Level3Data{
			Symbol:    "BTC/USD-" + strconv.Itoa(index),
			Type:      "snapshot",
			Timestamp: time.Unix(int64(index)+1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid", LimitPrice: 99, OrderQty: 1,
				Timestamp: time.Unix(int64(index)+1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask", LimitPrice: 101, OrderQty: 1,
				Timestamp: time.Unix(int64(index)+1, 0),
			}},
		}, 1, 8, book)
	}
}

func TestAnalyzerUsesManifoldPackage(t *testing.T) {
	Convey("Given the analyzer engine", t, func() {
		analyzer := NewAnalyzer(nil, nil)

		Convey("It should construct a manifold.Engine", func() {
			So(analyzer.engine, ShouldHaveSameTypeAs, manifold.NewEngine())
		})
	})
}
