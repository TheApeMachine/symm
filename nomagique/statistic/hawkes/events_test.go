package hawkes_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
window is what the Events node reports about the realisation it is holding.
*/
type window struct {
	times      []float64
	components []float64
	origin     float64
	horizon    float64
	span       float64
	count      float64
	counts     []float64
}

/*
arrival is one thing offered to the Events node, which it may retain or
refuse.
*/
type arrival struct {
	time      float64
	component float64
}

/*
observe offers a sequence of arrivals to one Events node and reports the
window it ends up holding.
*/
func observe(t *testing.T, dimension, capacity int32, arrivals []arrival) window {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Events_ServerToClient(hawkes.NewEvents())

	for _, offered := range arrivals {
		err := client.Write(ctx, func(params hawkes.Events_write_Params) error {
			params.SetTime(offered.time)
			params.SetComponent(offered.component)
			params.SetDimension(dimension)
			params.SetCapacity(capacity)
			return nil
		})

		if err != nil {
			t.Fatalf("events write: %v", err)
		}
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("events stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("events done: %v", err)
	}

	times, err := results.Times()

	if err != nil {
		t.Fatalf("events times: %v", err)
	}

	components, err := results.Components()

	if err != nil {
		t.Fatalf("events components: %v", err)
	}

	counts, err := results.Counts()

	if err != nil {
		t.Fatalf("events counts: %v", err)
	}

	held := window{
		times:   readList(times.Len(), times.At),
		counts:  readList(counts.Len(), counts.At),
		origin:  results.Origin(),
		horizon: results.Horizon(),
		span:    results.Span(),
		count:   results.Count(),
	}

	held.components = readList(components.Len(), components.At)

	return held
}

func TestEventsServer_Write(t *testing.T) {
	Convey("Given a node observing a two-component process", t, func() {
		Convey("When arrivals are offered in order", func() {
			held := observe(t, 2, 0, []arrival{
				{time: 1, component: 0},
				{time: 3, component: 1},
				{time: 7, component: 0},
			})

			Convey("Then the window spans the first arrival to the last", func() {
				So(held.times, ShouldResemble, []float64{1, 3, 7})
				So(held.components, ShouldResemble, []float64{0, 1, 0})
				So(held.origin, ShouldEqual, 1)
				So(held.horizon, ShouldEqual, 7)
				So(held.span, ShouldEqual, 6)
				So(held.count, ShouldEqual, 3)
				So(held.counts, ShouldResemble, []float64{2, 1})
			})
		})

		Convey("When an arrival predates the one before it", func() {
			held := observe(t, 2, 0, []arrival{
				{time: 5, component: 0},
				{time: 2, component: 1},
				{time: 6, component: 1},
			})

			Convey("Then it is refused rather than inserted behind the walk", func() {
				// The exponential kernel is carried forward in one pass, so
				// an arrival slipped in behind it would silently invalidate
				// every excitation already accumulated past its timestamp.
				So(held.times, ShouldResemble, []float64{5, 6})
				So(held.components, ShouldResemble, []float64{0, 1})
			})
		})

		Convey("When simultaneous arrivals are offered", func() {
			held := observe(t, 2, 0, []arrival{
				{time: 4, component: 0},
				{time: 4, component: 1},
			})

			Convey("Then both are retained, since neither precedes the other", func() {
				So(held.times, ShouldResemble, []float64{4, 4})
				So(held.components, ShouldResemble, []float64{0, 1})
				So(held.span, ShouldEqual, 0)
			})
		})

		Convey("When more arrivals are offered than the capacity holds", func() {
			held := observe(t, 2, 3, []arrival{
				{time: 1, component: 0},
				{time: 2, component: 1},
				{time: 3, component: 0},
				{time: 4, component: 1},
				{time: 5, component: 0},
			})

			Convey("Then the oldest are evicted and the window moves with them", func() {
				So(held.times, ShouldResemble, []float64{3, 4, 5})
				So(held.components, ShouldResemble, []float64{0, 1, 0})
				So(held.origin, ShouldEqual, 3)
				So(held.count, ShouldEqual, 3)
			})
		})

		Convey("When an arrival names a component outside the process", func() {
			held := observe(t, 2, 0, []arrival{
				{time: 1, component: 0},
				{time: 2, component: 4},
				{time: 3, component: -1},
			})

			Convey("Then it is refused rather than folded into a neighbour", func() {
				So(held.times, ShouldResemble, []float64{1})
				So(held.components, ShouldResemble, []float64{0})
			})
		})

		Convey("When nothing has arrived", func() {
			held := observe(t, 2, 0, nil)

			Convey("Then there is no window to report", func() {
				So(held.times, ShouldBeEmpty)
				So(held.count, ShouldEqual, 0)
				So(held.span, ShouldEqual, 0)
			})
		})
	})
}
