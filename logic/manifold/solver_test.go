package manifold

import (
	"context"
	"sync"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/tests/market"
)

/* solverBook applies the shared market tape through the venue's real book. */
type solverBook struct {
	books sync.Map
}

func (source *solverBook) Books() *sync.Map { return &source.books }

func (source *solverBook) Book(symbol string, read func(*spotbook.Book)) {
	book, found := source.books.Load(symbol)

	if found {
		read(book.(*spotbook.Book))
	}
}

func (source *solverBook) Apply(message kraken.Level3Data) {
	value, _ := source.books.LoadOrStore(message.Symbol, spotbook.New())
	book := value.(*spotbook.Book)

	for _, side := range []struct {
		direction spotbook.BookDirection
		orders    []kraken.Level3Order
	}{{spotbook.Bid, message.Bids}, {spotbook.Ask, message.Asks}} {
		for _, order := range side.orders {
			book.Update(&spotbook.UpdateOptions{Direction: side.direction,
				ID: order.OrderID, Price: order.LimitPrice, Quantity: order.OrderQty})
		}
	}
}

func TestSolverAdvance(t *testing.T) {
	Convey("Given a resident Metal solver receiving the shared multi-leg market tape", t, func() {
		physics := sensorium.NewManifold(8, 8, 8)
		So(physics, ShouldNotBeNil)
		ctx, cancel := context.WithCancel(t.Context())
		books := &solverBook{}
		solver := &Solver{ctx: ctx, cancel: cancel, physics: physics,
			dataset: NewDataset(), books: books, loaded: make(map[int64]struct{})}
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		tape := market.NewLevel3Tape("TEST/USD", time.Unix(1, 0))
		var previous *State
		var saved sensorium.State
		var modes []WaveMode

		for _, message := range tape.Messages {
			books.Apply(message)
			reading := solver.Advance()
			So(solver.Error(), ShouldBeNil)
			So(reading, ShouldNotBeNil)
			So(reading.N, ShouldBeGreaterThan, 0)
			So(reading.TokenIDs, ShouldHaveLength, reading.N)
			So(reading.Energy, ShouldHaveLength, reading.N)
			So(reading.Heat, ShouldHaveLength, reading.N)
			So(reading.Vel, ShouldHaveLength, reading.N*3)
			So(reading.Modes, ShouldNotBeEmpty)
			So(reading.MomRho, ShouldBeNil)
			So(reading.FieldEnergy, ShouldBeNil)
			So(reading.WaveReal, ShouldBeNil)
			So(reading.WaveImag, ShouldBeNil)
			So(solver.Reading(), ShouldEqual, reading)

			if previous != nil {
				So(previous.State, ShouldResemble, saved)
				So(previous.Modes, ShouldResemble, modes)
				So(reading.Version, ShouldEqual, previous.Version+1)
			}

			previous, saved = reading, cloneState(&reading.State)
			modes = append([]WaveMode(nil), reading.Modes...)
		}

		snapshot := solver.Snapshot()
		So(snapshot, ShouldNotBeNil)
		So(snapshot.State, ShouldResemble, previous.State)
		So(snapshot.Modes, ShouldResemble, previous.Modes)
		So(snapshot.Version, ShouldEqual, previous.Version)
		So(snapshot.MomRho, ShouldHaveLength, 8*8*8*4)
		So(snapshot.WaveReal, ShouldHaveLength, 8*8*8)
		So(previous.MomRho, ShouldBeNil)
	})
}

func BenchmarkSolverAdvance(b *testing.B) {
	physics := sensorium.NewManifold(8, 8, 8)

	if physics == nil {
		b.Fatal("Metal domain unavailable")
	}

	ctx, cancel := context.WithCancel(b.Context())
	books := &solverBook{}
	solver := &Solver{ctx: ctx, cancel: cancel, physics: physics,
		dataset: NewDataset(), books: books, loaded: make(map[int64]struct{})}
	defer solver.Close()
	tape := market.NewLevel3Tape("TEST/USD", time.Unix(1, 0))

	for _, message := range tape.Messages {
		books.Apply(message)
	}

	b.ReportAllocs()

	for b.Loop() {
		if solver.Advance() == nil {
			b.Fatal(solver.Error())
		}
	}
}
