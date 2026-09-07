package temporal_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
	"time"
)

func TestVelocityNext(t *testing.T) {
	node := temporal.NewVelocity(store.NewGet("value"), store.NewGet("at"))
	origin := time.Unix(1700000000, 0).UnixNano()
	inputs := []core.Primitive{
		tests.Record(map[string]any{"value": 10.0, "at": origin}),
		tests.Record(map[string]any{"value": 13.0, "at": origin + int64(1000000000)}),
		tests.Record(map[string]any{"value": 16.0, "at": origin + int64(1000000000)}),
		tests.Record(map[string]any{"value": 17.0, "at": origin + int64(1000000001)}),
	}
	output := tests.Drain(t, node, tests.Values(inputs...))
	tests.Sound(t, node)
	if len(output) != 4 {
		t.Fatal(output)
	}
	for index, want := range []float64{0, 3, 0, 1e9} {
		tests.EqualNumber(t, tests.Number(t, tests.Fields(t, output[index]), "rate"), want)
	}
	next := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"value": 18.0, "at": origin + int64(2000000001)})))
	tests.EqualNumber(t, tests.Number(t, tests.Fields(t, next[0]), "rate"), 1)
}

func TestVelocityObserve(t *testing.T) {
	Convey("Given a finite difference with one previous observation", t, func() {
		velocity := &temporal.Velocity{}
		first := velocity.Observe(10, 0)
		So(first.HasPrior, ShouldBeFalse)
		So(first.Defined, ShouldBeFalse)
		for _, fixture := range []struct {
			value   float64
			at      int64
			rate    float64
			defined bool
		}{
			{13, int64(time.Second), 3, true},
			{16, int64(time.Second), 0, false},
			{17, int64(time.Second) + 1, 1e9, true},
			{11, int64(time.Second), 0, false},
			{8, int64(2 * time.Second), -3, true},
		} {
			reading := velocity.Observe(fixture.value, fixture.at)
			tests.EqualNumber(t, reading.Rate, fixture.rate)
			So(reading.Defined, ShouldEqual, fixture.defined)
			So(reading.HasPrior, ShouldBeTrue)
		}
		So(first.Through.Value, ShouldEqual, 10)
		So(first.HasPrior, ShouldBeFalse)
	})
}

func BenchmarkVelocityNext(b *testing.B) {
	node := temporal.NewVelocity(store.NewGet("value"), store.NewGet("at"))
	input := tests.Values(tests.Record(map[string]any{"value": 13.0, "at": int64(time.Second)}))
	b.ReportAllocs()
	for b.Loop() {
		if node.Next(input) == nil || node.Next(input) != nil {
			b.Fatal("expected one velocity reading")
		}
		if err := node.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
