package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestSQLiteListLifecycleEvents(t *testing.T) {
	Convey("Given one position with independently correlated execution instructions", t, func() {
		engine, err := NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		Reset(func() { So(engine.Close(), ShouldBeNil) })
		So(engine.WriteRun(hindsight.Run{ID: "run", StartedAt: time.Now()}), ShouldBeNil)
		for _, identity := range []string{"entry", "reduction", "exit"} {
			So(engine.WriteLifecycleEvent("run", hindsight.LifecycleEvent{
				DecisionID: "entry", ActionCorrelationID: identity, Symbol: "TEST/USD", Kind: "execution_terminal", At: time.Now(),
				Execution: &hindsight.ExecutionFact{ClientOrderID: identity, OrderStatus: "filled", AvgPrice: "100", CumQty: "1"},
			}), ShouldBeNil)
		}
		events, err := engine.ListLifecycleEvents("run")
		So(err, ShouldBeNil)
		So(len(events), ShouldEqual, 3)
		for index, identity := range []string{"entry", "reduction", "exit"} {
			So(events[index].DecisionID, ShouldEqual, "entry")
			So(events[index].ActionCorrelationID, ShouldEqual, identity)
		}
	})
}

func BenchmarkSQLiteListLifecycleEvents(b *testing.B) {
	engine, err := NewSQLite(b.TempDir() + "/events.sqlite")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := engine.Close(); err != nil {
			b.Error(err)
		}
	})
	if err := engine.WriteRun(hindsight.Run{ID: "run", StartedAt: time.Now()}); err != nil {
		b.Fatal(err)
	}
	for _, identity := range []string{"entry", "reduction", "exit"} {
		if err := engine.WriteLifecycleEvent("run", hindsight.LifecycleEvent{
			DecisionID: "entry", ActionCorrelationID: identity, Symbol: "TEST/USD", Kind: "execution_terminal", At: time.Now(),
			Execution: &hindsight.ExecutionFact{ClientOrderID: identity, OrderStatus: "filled", AvgPrice: "100", CumQty: "1"},
		}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.ListLifecycleEvents("run"); err != nil {
			b.Fatal(err)
		}
	}
}
