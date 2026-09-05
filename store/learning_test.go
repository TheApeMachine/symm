package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestWriterWriteLearning(t *testing.T) {
	Convey("Given the canonical ordered SQLite writer", t, func() {
		repository, err := NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		sequencer, err := hindsight.NewSequencer("run-1")
		So(err, ShouldBeNil)
		writer, err := NewWriter(repository, sequencer, testWriterQueueDepth, testWriterBatchSize)
		So(err, ShouldBeNil)
		Reset(func() { So(writer.Close(), ShouldBeNil); So(repository.Close(), ShouldBeNil) })

		Convey("Issued, filled and resolved events retain their frozen identities", func() {
			event := hindsight.LearningEvent{ID: 1, Symbol: "TEST/USD", At: time.Unix(1, 0), Kind: "issued", Context: []uint64{3, 2, 1}}
			So(writer.WriteLearning("run-1", event), ShouldBeNil)
			event.Context[0] = 999
			event.Kind, event.At = "filled", time.Unix(2, 0)
			So(writer.WriteLearning("run-1", event), ShouldBeNil)
			event.Kind, event.At = "resolved", time.Unix(3, 0)
			So(writer.WriteLearning("run-1", event), ShouldBeNil)
			So(writer.Sync(), ShouldBeNil)
			events, err := repository.LearningEvents("run-1", "TEST/USD", 3)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 3)
			So(events[0].Kind, ShouldEqual, "resolved")
			So(events[2].Context, ShouldResemble, []uint64{3, 2, 1})
		})

		Convey("Missing record identity is rejected before enqueue", func() {
			So(writer.WriteLearning("run-1", hindsight.LearningEvent{}), ShouldNotBeNil)
		})
	})
}

func TestSQLiteLearningEvents(t *testing.T) {
	Convey("Given interleaved symbols and runs in the existing event journal", t, func() {
		repository, err := NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		sequencer, err := hindsight.NewSequencer("run-1")
		So(err, ShouldBeNil)
		writer, err := NewWriter(repository, sequencer, testWriterQueueDepth, testWriterBatchSize)
		So(err, ShouldBeNil)
		Reset(func() { So(writer.Close(), ShouldBeNil); So(repository.Close(), ShouldBeNil) })

		for index, symbol := range []string{"TEST/USD", "ANOTHER/USD", "ANOTHER/USD"} {
			So(writer.WriteLearning("run-1", hindsight.LearningEvent{ID: uint64(index + 1), Symbol: symbol, At: time.Unix(int64(index+1), 0)}), ShouldBeNil)
		}
		So(writer.WriteLearning("run-2", hindsight.LearningEvent{ID: 4, Symbol: "TEST/USD", At: time.Unix(4, 0)}), ShouldBeNil)
		So(writer.Sync(), ShouldBeNil)

		Convey("The page limit applies after symbol and run selection", func() {
			events, err := repository.LearningEvents("run-1", "TEST/USD", 1)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 1)
			So(events[0].ID, ShouldEqual, 1)
		})
	})
}

func BenchmarkWriterWriteLearning(b *testing.B) {
	repository, err := NewSQLite(b.TempDir() + "/events.sqlite")
	if err != nil {
		b.Fatal(err)
	}
	sequencer, err := hindsight.NewSequencer("benchmark")
	if err != nil {
		b.Fatal(err)
	}
	writer, err := NewWriter(repository, sequencer, testWriterQueueDepth, testWriterBatchSize)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := writer.Close(); err != nil {
			b.Error(err)
		}
		if err := repository.Close(); err != nil {
			b.Error(err)
		}
	})
	event := hindsight.LearningEvent{ID: 1, Symbol: "TEST/USD", At: time.Unix(1, 0), Kind: "issued", Context: []uint64{1, 2, 3}}
	b.ReportAllocs()

	for b.Loop() {
		event.ID++
		if err := writer.WriteLearning("benchmark", event); err != nil {
			b.Fatal(err)
		}
	}

	if err := writer.Sync(); err != nil {
		b.Fatal(err)
	}
}
