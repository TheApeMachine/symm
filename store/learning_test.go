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
			events, err := repository.LearningEvents("run-1", "TEST/USD", "", 3)
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
			events, err := repository.LearningEvents("run-1", "TEST/USD", "", 1)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 1)
			So(events[0].ID, ShouldEqual, 1)
		})
		Convey("Prospective candidate input and later outcome remain separate across symbols", func() {
			event := hindsight.LearningEvent{ID: 10, Symbol: "TEST/USD", At: time.Unix(10, 0), Kind: "candidate", CandidateID: "candidate-1", Candidate: &hindsight.CandidateRecord{ID: "candidate-1", Context: []uint64{3, 2, 1}, AccountCash: "150"}}
			So(writer.WriteLearning("run-1", event), ShouldBeNil)
			event.Candidate.Context[0] = 999
			result := hindsight.LearningEvent{ID: 10, Symbol: "account", At: time.Unix(12, 0), Kind: "portfolio_resolved", CandidateID: "candidate-1", PortfolioID: "allocation-1", Target: 0.01}
			So(writer.WriteLearning("run-1", result), ShouldBeNil)
			So(writer.Sync(), ShouldBeNil)
			events, err := repository.LearningEvents("run-1", "", "candidate-1", 10)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 2)
			So(events[0].PortfolioID, ShouldEqual, "allocation-1")
			So(events[1].Candidate.Context, ShouldResemble, []uint64{3, 2, 1})
			So(events[1].Candidate.AccountCash, ShouldEqual, "150")
			So(events[1].PortfolioID, ShouldEqual, "")
		})

	})
}

func TestSQLiteLearningExperiences(t *testing.T) {
	Convey("Given reused model ticket numbers in different runs", t, func() {
		repository, err := NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		sequencer, err := hindsight.NewSequencer("one")
		So(err, ShouldBeNil)
		writer, err := NewWriter(repository, sequencer, testWriterQueueDepth, testWriterBatchSize)
		So(err, ShouldBeNil)
		Reset(func() { So(writer.Close(), ShouldBeNil); So(repository.Close(), ShouldBeNil) })
		at := time.Unix(100, 0)
		So(writer.WriteLearning("one", hindsight.LearningEvent{ID: 1, Symbol: "A/USD", Kind: "issued", At: at, Authority: 1, Action: "enter"}), ShouldBeNil)
		So(writer.WriteLearning("two", hindsight.LearningEvent{ID: 1, Symbol: "B/USD", Kind: "issued", At: at, Authority: 1, Action: "hold"}), ShouldBeNil)
		for index := range 20 {
			So(writer.WriteLearning("one", hindsight.LearningEvent{ID: uint64(index + 2), Symbol: "A/USD", Kind: "filled", At: at}), ShouldBeNil)
		}
		So(writer.WriteLearning("one", hindsight.LearningEvent{ID: 1, Symbol: "A/USD", Kind: "resolved", At: at.Add(time.Second), Target: 0.1}), ShouldBeNil)
		So(writer.WriteLearning("two", hindsight.LearningEvent{ID: 2, Symbol: "B/USD", Kind: "resolved", At: at.Add(time.Second), Target: 99}), ShouldBeNil)
		So(writer.Sync(), ShouldBeNil)
		events, err := repository.LearningExperiences("resolved", 1)
		So(err, ShouldBeNil)
		So(events, ShouldHaveLength, 2)
		So(events[0].Run, ShouldEqual, hindsight.RunID("one"))
		So(events[0].Kind, ShouldEqual, "issued")
		So(events[1].Kind, ShouldEqual, "resolved")
		So(events[1].Target, ShouldEqual, 0.1)
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

func BenchmarkSQLiteLearningExperiences(b *testing.B) {
	repository, err := NewSQLite(b.TempDir() + "/events.sqlite")

	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := repository.Close(); err != nil {
			b.Error(err)
		}
	})
	// A thousand local pairs per portfolio pair reproduces the retained
	// journal's sparse capital events without timing setup as warmup work.
	_, err = repository.database.Exec(`WITH RECURSIVE sequence(value) AS (
		SELECT 1 UNION ALL SELECT value+1 FROM sequence WHERE value<10000
	) INSERT INTO learning_events(run_id,symbol,at,data)
	SELECT 'benchmark','account','1970-01-01T00:00:01Z',CAST(json_object(
		'id',value,'symbol','account','at','1970-01-01T00:00:01Z',
		'kind',CASE WHEN value%1000=0 THEN 'portfolio_issued' ELSE 'issued' END) AS BLOB)
	FROM sequence`)

	if err != nil {
		b.Fatal(err)
	}
	_, err = repository.database.Exec(`INSERT INTO learning_events(run_id,symbol,at,data)
		SELECT run_id,symbol,at,CAST(json_set(data,'$.kind',
		CASE WHEN json_extract(data,'$.kind')='portfolio_issued'
		THEN 'portfolio_resolved' ELSE 'resolved' END) AS BLOB) FROM learning_events`)

	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		events, err := repository.LearningExperiences("portfolio_resolved", 10)

		if err != nil {
			b.Fatal(err)
		}

		if len(events) != 20 {
			b.Fatalf("expected 10 complete portfolio pairs, got %d events", len(events))
		}
	}
}
