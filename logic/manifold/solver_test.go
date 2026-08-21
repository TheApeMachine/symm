package manifold

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/adaptive"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
countingBookSource exposes how many complete-universe loads a work cut causes.
*/
type countingBookSource struct {
	reads atomic.Uint64
}

/*
residentBookSource exposes one complete Level 3 book to oscillator tests.
*/
type residentBookSource struct {
	resident *mgrbook.Book
}

func (source *countingBookSource) Book(_ string, read func(*mgrbook.Book)) {
	source.reads.Add(1)
	read(nil)
}

func (source *residentBookSource) Book(_ string, read func(*mgrbook.Book)) {
	read(source.resident)
}

func bookOscillatorSolver(resident *mgrbook.Book) *Solver {
	return &Solver{
		api: &residentBookSource{resident: resident},
		config: pmanifold.Config{
			DomainX:  2,
			DomainY:  3,
			DomainZ:  4,
			DeltaT:   1,
			MaxModes: 4,
		},
		scales:    make(map[string]*adaptive.Accumulator),
		converged: make(map[string]float64),
		priorPos:  make(map[string]map[string][3]float64),
	}
}

func residentBook(
	updates []mgrbook.UpdateOptions,
	descendingQueue bool,
) *mgrbook.Book {
	resident := mgrbook.New()

	for index := range updates {
		resident.Update(&updates[index])
	}

	for _, side := range []*mgrbook.Side{resident.Bids, resident.Asks} {
		for _, level := range side.Levels {
			queue := level.Queue()
			sort.Slice(queue, func(left, right int) bool {
				if descendingQueue {
					return queue[left].ID > queue[right].ID
				}

				return queue[left].ID < queue[right].ID
			})
		}
	}

	return resident
}

func oscillatorBits(oscillator pmanifold.Oscillator) [10]uint64 {
	return [10]uint64{
		math.Float64bits(oscillator.Phase),
		math.Float64bits(oscillator.Omega),
		math.Float64bits(oscillator.Amplitude),
		math.Float64bits(oscillator.PosX),
		math.Float64bits(oscillator.PosY),
		math.Float64bits(oscillator.PosZ),
		math.Float64bits(oscillator.Heat),
		math.Float64bits(oscillator.VelX),
		math.Float64bits(oscillator.VelY),
		math.Float64bits(oscillator.VelZ),
	}
}

func oscillatorSequenceBits(oscillators []pmanifold.Oscillator) [][10]uint64 {
	bits := make([][10]uint64, len(oscillators))

	for index, oscillator := range oscillators {
		bits[index] = oscillatorBits(oscillator)
	}

	return bits
}

func oscillatorBookUpdates() []mgrbook.UpdateOptions {
	at := time.Unix(1, 0).UTC()

	return []mgrbook.UpdateOptions{
		{Direction: mgrbook.Bid, ID: "bid-alpha", Price: decimal.NewFromInt64(99), Quantity: decimal.NewFromInt64(1), Timestamp: at},
		{Direction: mgrbook.Bid, ID: "bid-beta", Price: decimal.NewFromInt64(99), Quantity: decimal.NewFromInt64(4), Timestamp: at},
		{Direction: mgrbook.Ask, ID: "ask-alpha", Price: decimal.NewFromInt64(101), Quantity: decimal.NewFromInt64(2), Timestamp: at},
		{Direction: mgrbook.Ask, ID: "ask-beta", Price: decimal.NewFromInt64(101), Quantity: decimal.NewFromInt64(8), Timestamp: at},
	}
}

func oscillatorBenchmarkBook(orderCount int) *mgrbook.Book {
	updates := make([]mgrbook.UpdateOptions, 0, orderCount)
	at := time.Unix(1, 0).UTC()
	half := orderCount / 2

	for index := range half {
		updates = append(updates, mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        fmt.Sprintf("bid-%03d", index),
			Price:     decimal.NewFromInt64(int64(1000 - index)),
			Quantity:  decimal.NewFromInt64(int64(index + 1)),
			Timestamp: at.Add(time.Duration(index)),
		})
	}

	for index := half; index < orderCount; index++ {
		updates = append(updates, mgrbook.UpdateOptions{
			Direction: mgrbook.Ask,
			ID:        fmt.Sprintf("ask-%03d", index),
			Price:     decimal.NewFromInt64(int64(1001 + index - half)),
			Quantity:  decimal.NewFromInt64(int64(index + 1)),
			Timestamp: at.Add(time.Duration(index)),
		})
	}

	return residentBook(updates, false)
}

func consumeTestSolver(
	ctx context.Context,
) (*Solver, *types.Thesis, *countingBookSource, *atomic.Bool) {
	thesis := types.NewThesis(ctx, nil)
	source := &countingBookSource{}
	enabled := &atomic.Bool{}
	solver := &Solver{
		ctx:      ctx,
		thesis:   thesis,
		api:      source,
		physics:  &pmanifold.Solver{},
		universe: []string{"BTC/USD"},
	}
	solver.work = transport.NewConsumer[*types.Symbol](solver.Name(), func() {
		if enabled.Load() {
			solver.consume()
		}
	})
	thesis.Work(types.SourceManifold).Register(solver.work)

	return solver, thesis, source, enabled
}

func waitForConsume(
	ctx context.Context,
	work *transport.MapReduce[*types.Symbol],
) error {
	for !work.Idle() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			runtime.Gosched()
		}
	}

	return nil
}

func TestAdmit(t *testing.T) {
	Convey("Given a manifold carrier lattice at capacity", t, func() {
		solver := &Solver{}

		for index := range phaseLatticeWidth {
			So(solver.admit(fmt.Sprintf("SYMBOL-%d", index)), ShouldBeTrue)
		}

		Convey("It should retain the resident universe and reject another carrier", func() {
			So(solver.admit("OVERFLOW"), ShouldBeFalse)
			So(solver.universe, ShouldHaveLength, int(phaseLatticeWidth))
			So(solver.admit("SYMBOL-0"), ShouldBeTrue)
			So(solver.universe, ShouldHaveLength, int(phaseLatticeWidth))
		})
	})
}

func TestBookOscillators(t *testing.T) {
	Convey("Given logically identical books with opposite resident order", t, func() {
		ascendingUpdates := oscillatorBookUpdates()
		descendingUpdates := append([]mgrbook.UpdateOptions(nil), ascendingUpdates...)

		for left, right := 0, len(descendingUpdates)-1; left < right; left, right = left+1, right-1 {
			descendingUpdates[left], descendingUpdates[right] =
				descendingUpdates[right], descendingUpdates[left]
		}

		ascendingBook := residentBook(ascendingUpdates, false)
		descendingBook := residentBook(descendingUpdates, true)
		So(ascendingBook.BestBid().Queue()[0].ID, ShouldEqual, "bid-alpha")
		So(descendingBook.BestBid().Queue()[0].ID, ShouldEqual, "bid-beta")
		ascendingSolver := bookOscillatorSolver(ascendingBook)
		descendingSolver := bookOscillatorSolver(descendingBook)
		at := time.Unix(2, 0).UTC()

		ascendingOscillators, err := ascendingSolver.bookOscillators(
			"BTC/USD", 0.25, 0.5, at,
		)
		So(err, ShouldBeNil)
		descendingOscillators, err := descendingSolver.bookOscillators(
			"BTC/USD", 0.25, 0.5, at,
		)
		So(err, ShouldBeNil)

		Convey("The canonical projection should be bit-identical", func() {
			So(ascendingOscillators, ShouldHaveLength, 4)
			So(
				oscillatorSequenceBits(descendingOscillators),
				ShouldResemble,
				oscillatorSequenceBits(ascendingOscillators),
			)
			So(
				math.Float64bits(descendingSolver.converged["BTC/USD"]),
				ShouldEqual,
				math.Float64bits(ascendingSolver.converged["BTC/USD"]),
			)
			So(
				math.Float64bits(ascendingOscillators[0].PosY),
				ShouldEqual,
				math.Float64bits(0),
			)
			So(
				math.Float64bits(ascendingOscillators[3].PosY),
				ShouldEqual,
				math.Float64bits(ascendingSolver.config.DomainY),
			)
		})
	})
}

func TestConsume(t *testing.T) {
	Convey("Given one bounded burst of manifold readiness notifications", t, func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		solver, thesis, source, enabled := consumeTestSolver(ctx)
		work := thesis.Work(types.SourceManifold)

		for range phaseLatticeWidth {
			thesis.ScheduleWork(types.SourceManifold, nil)
		}

		enabled.Store(true)
		solver.consume()
		err := waitForConsume(ctx, work)

		Convey("It should settle the universe once without losing a request", func() {
			So(err, ShouldBeNil)
			So(solver.Error(), ShouldBeNil)
			So(source.reads.Load(), ShouldEqual, uint64(1))
			So(solver.requested, ShouldEqual, uint64(phaseLatticeWidth))
			So(solver.completed, ShouldEqual, uint64(phaseLatticeWidth))
		})

		Convey("A later notification should cause exactly one later settlement", func() {
			thesis.ScheduleWork(types.SourceManifold, nil)
			So(waitForConsume(ctx, work), ShouldBeNil)
			So(source.reads.Load(), ShouldEqual, uint64(2))
			So(solver.requested, ShouldEqual, uint64(phaseLatticeWidth+1))
			So(solver.completed, ShouldEqual, uint64(phaseLatticeWidth+1))
		})
	})
}

func BenchmarkConsume(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	solver, thesis, _, enabled := consumeTestSolver(ctx)
	work := thesis.Work(types.SourceManifold)
	b.ReportAllocs()

	for b.Loop() {
		enabled.Store(false)

		for range phaseLatticeWidth {
			thesis.ScheduleWork(types.SourceManifold, nil)
		}

		enabled.Store(true)
		solver.consume()

		if err := waitForConsume(ctx, work); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBookOscillators(b *testing.B) {
	// The live manifold capacity documents about 68 visible orders per symbol.
	const visibleOrderCount = 68
	solver := bookOscillatorSolver(oscillatorBenchmarkBook(visibleOrderCount))
	at := time.Unix(2, 0).UTC()
	b.ReportAllocs()

	for b.Loop() {
		oscillators, err := solver.bookOscillators(
			"BTC/USD", 0.25, 0.5, at,
		)

		if err != nil {
			b.Fatal(err)
		}

		if len(oscillators) != visibleOrderCount {
			b.Fatalf("got %d oscillators, want %d", len(oscillators), visibleOrderCount)
		}
	}
}
