package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

const (
	testWriterQueueDepth = 16
	testWriterBatchSize  = 8
)

func TestWriterCapture(t *testing.T) {
	Convey("Given a writer over a recording repository", t, func() {
		repository := newRecordingRepository()
		sequencer, err := hindsight.NewSequencer("run-1")
		So(err, ShouldBeNil)

		writer, err := NewWriter(
			repository, sequencer, testWriterQueueDepth, testWriterBatchSize,
		)
		So(err, ShouldBeNil)
		Reset(func() { _ = writer.Close() })

		Convey("Capturing a frame mints an identity and queues its persistence", func() {
			identity, err := writer.Capture("ticker", "wss://example", []byte("raw"), time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})

			So(err, ShouldBeNil)
			So(identity.Valid(), ShouldBeTrue)
			So(writer.Sync(), ShouldBeNil)
			So(repository.captures, ShouldHaveLength, 1)
			So(repository.captures[0].identity, ShouldResemble, identity)
			So(repository.captures[0].kind, ShouldEqual, "ticker")
			So(repository.captures[0].endpoint, ShouldEqual, "wss://example")

			// The payload is the exact bytes off the wire; nothing is prefixed.
			So(string(repository.captures[0].payload), ShouldEqual, "raw")
		})

		Convey("Two frames earn distinct run-local capture sequences", func() {
			first, err := writer.Capture("ticker", "wss://example", []byte("1"), time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})
			So(err, ShouldBeNil)

			second, err := writer.Capture("ticker", "wss://example", []byte("2"), time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})
			So(err, ShouldBeNil)

			So(second.Sequence, ShouldBeGreaterThan, first.Sequence)
			So(second.Run, ShouldEqual, first.Run)
		})
	})

	Convey("Given a nil sequencer", t, func() {
		Convey("NewWriter rejects it", func() {
			writer, err := NewWriter(
				newRecordingRepository(), nil, testWriterQueueDepth, testWriterBatchSize,
			)
			So(writer, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a writer with a nil repository", t, func() {
		sequencer, _ := hindsight.NewSequencer("run-1")
		writer, err := NewWriter(nil, sequencer, testWriterQueueDepth, testWriterBatchSize)
		So(err, ShouldBeNil)
		Reset(func() { _ = writer.Close() })

		Convey("Capture still mints an identity without persisting", func() {
			identity, err := writer.Capture("ticker", "x", []byte("y"), time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})
			So(err, ShouldBeNil)
			So(identity.Valid(), ShouldBeTrue)
		})
	})
}

func TestWriterReconnectTest(t *testing.T) {
	Convey("Given a writer minting on one endpoint", t, func() {
		repository := newRecordingRepository()
		sequencer, _ := hindsight.NewSequencer("run-1")
		writer, _ := NewWriter(
			repository, sequencer, testWriterQueueDepth, testWriterBatchSize,
		)
		Reset(func() { _ = writer.Close() })

		before, err := writer.Capture("ticker", "wss://example", []byte("a"), time.Now(), hindsight.StreamRef{
			Stream:   hindsight.Stream("wss://example:test"),
			Epoch:    1,
			Sequence: 1,
		})
		So(err, ShouldBeNil)

		Convey("Hindsight records the transport-minted epoch, never supplies it", func() {
			// The transport advanced the epoch on reconnect and mints the next
			// frame with Epoch 2; the writer copies that fact verbatim.
			after, err := writer.Capture("ticker", "wss://example", []byte("b"), time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:b"),
				Epoch:    2,
				Sequence: 1,
			})
			So(err, ShouldBeNil)

			So(after.StreamEpoch, ShouldEqual, before.StreamEpoch+1)
			So(after.StreamSequence, ShouldEqual, uint64(1))
			So(after.Sequence, ShouldBeGreaterThan, before.Sequence)
		})

		Convey("an absent ref leaves the sequencer's own bookkeeping intact", func() {
			// A zero ref means the caller did not mint operational metadata; the
			// writer keeps its own epoch/sequence count as a fallback.
			after, err := writer.Capture(
				"ticker", "wss://example", []byte("c"), time.Now(), hindsight.StreamRef{},
			)
			So(err, ShouldBeNil)
			So(after.StreamEpoch, ShouldEqual, before.StreamEpoch)
		})
	})
}

func TestWriterCaptureFailureTest(t *testing.T) {
	Convey("Given a writer whose repository fails to persist", t, func() {
		repository := &failingRepository{}
		sequencer, _ := hindsight.NewSequencer("run-1")
		writer, _ := NewWriter(
			repository, sequencer, testWriterQueueDepth, testWriterBatchSize,
		)
		Reset(func() { _ = writer.Close() })

		Convey("Capture returns its identity before persistence and Sync exposes the failure", func() {
			identity, err := writer.Capture("ticker", "wss://example", []byte("x"), time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})

			So(err, ShouldBeNil)
			So(identity.Valid(), ShouldBeTrue)
			So(writer.Sync(), ShouldNotBeNil)
			So(writer.Error(), ShouldNotBeNil)

			So(repository.gaps, ShouldHaveLength, 1)
			So(repository.gaps[0].runID, ShouldEqual, hindsight.RunID("run-1"))
			So(repository.gaps[0].encoding, ShouldEqual, "capture_persistence_failure")
			So(repository.gaps[0].detail, ShouldContainSubstring, "boom")
		})
	})
}

func TestWriterCaptureDoesNotWaitForRepository(t *testing.T) {
	Convey("Given a repository whose capture write is blocked", t, func() {
		repository := &blockingRepository{
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		sequencer, err := hindsight.NewSequencer("run-1")
		So(err, ShouldBeNil)
		writer, err := NewWriter(
			repository, sequencer, testWriterQueueDepth, testWriterBatchSize,
		)
		So(err, ShouldBeNil)
		Reset(func() {
			repository.unblock()
			_ = writer.Close()
		})

		Convey("Capture returns after ordered acceptance without waiting for storage", func() {
			identity, err := writer.Capture(
				"ticker",
				"wss://example",
				[]byte("raw"),
				time.Now(),
				hindsight.StreamRef{Stream: "public:ticker", Epoch: 1, Sequence: 1},
			)

			So(err, ShouldBeNil)
			So(identity.Valid(), ShouldBeTrue)
			<-repository.started
			repository.unblock()
			So(writer.Sync(), ShouldBeNil)
		})
	})
}

func TestWriterWriteLifecycle(t *testing.T) {
	Convey("Given a SQLite-backed capture writer", t, func() {
		repository, err := NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		sequencer, err := hindsight.NewSequencer("run-1")
		So(err, ShouldBeNil)
		writer, err := NewWriter(
			repository, sequencer, testWriterQueueDepth, testWriterBatchSize,
		)
		So(err, ShouldBeNil)
		Reset(func() {
			_ = writer.Close()
			_ = repository.Close()
		})

		Convey("a broker lifecycle event reaches SQLite through the ordered queue", func() {
			So(writer.WriteLifecycle("run-1", hindsight.LifecycleEvent{
				DecisionID: "decision-1",
				Symbol:     "TEST/USD",
				Kind:       "entry_fill",
				At:         time.Unix(1, 0),
			}), ShouldBeNil)
			So(writer.Sync(), ShouldBeNil)

			var count int
			err := repository.database.QueryRow(
				"SELECT count(*) FROM lifecycle WHERE decision_id = ?",
				"decision-1",
			).Scan(&count)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})
	})
}

/*
failingRepository always fails WriteCapture so the writer's gap-marking path is
exercised directly.
*/
type failingRepository struct {
	gaps []recordedGap
}

type blockingRepository struct {
	recordingRepository
	started     chan struct{}
	release     chan struct{}
	startedOnce bool
	releaseOnce bool
}

func (repo *blockingRepository) WriteCapture(
	identity hindsight.CaptureIdentity,
	endpoint, kind string,
	payload []byte,
	at time.Time,
) error {
	if !repo.startedOnce {
		repo.startedOnce = true
		close(repo.started)
	}

	<-repo.release

	return repo.recordingRepository.WriteCapture(identity, endpoint, kind, payload, at)
}

func (repo *blockingRepository) unblock() {
	if repo.releaseOnce {
		return
	}

	repo.releaseOnce = true
	close(repo.release)
}

func (repo *failingRepository) WriteRun(run hindsight.Run) error { return nil }
func (repo *failingRepository) WriteFrame(endpoint, kind string, payload []byte, at time.Time) error {
	return nil
}
func (repo *failingRepository) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	return nil
}
func (repo *failingRepository) WriteWitness(witness hindsight.ArtifactWitness) error {
	return nil
}
func (repo *failingRepository) Close() error { return nil }

func (repo *failingRepository) WriteCapture(
	identity hindsight.CaptureIdentity,
	endpoint, kind string,
	payload []byte,
	at time.Time,
) error {
	return errnie.Error(errnie.Err(errnie.IO, "boom", nil))
}

func (repo *failingRepository) MarkGapped(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
) error {
	repo.gaps = append(repo.gaps, recordedGap{
		runID:    runID,
		sequence: sequence,
		encoding: encoding,
		detail:   detail,
	})

	return nil
}

func (repo *failingRepository) MarkCorrupt(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
) error {
	return repo.MarkGapped(runID, sequence, encoding, detail)
}

/*
recordingRepository is a minimal in-memory Repository used to assert the writer
records the right identity, kind, endpoint, and payload without a SQLite file.
*/
type recordingRepository struct {
	captures []recordedCapture
	gaps     []recordedGap
}

type recordedGap struct {
	runID    hindsight.RunID
	sequence hindsight.CaptureSequence
	encoding string
	detail   string
}

type recordedCapture struct {
	identity hindsight.CaptureIdentity
	endpoint string
	kind     string
	payload  []byte
}

func newRecordingRepository() *recordingRepository {
	return &recordingRepository{}
}

func (repo *recordingRepository) WriteRun(run hindsight.Run) error { return nil }

func (repo *recordingRepository) WriteCapture(
	identity hindsight.CaptureIdentity,
	endpoint, kind string,
	payload []byte,
	at time.Time,
) error {
	repo.captures = append(repo.captures, recordedCapture{
		identity: identity,
		endpoint: endpoint,
		kind:     kind,
		payload:  payload,
	})

	return nil
}

func (repo *recordingRepository) WriteFrame(endpoint, kind string, payload []byte, at time.Time) error {
	return nil
}

func (repo *recordingRepository) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	return nil
}

func (repo *recordingRepository) WriteWitness(witness hindsight.ArtifactWitness) error {
	return nil
}

func (repo *recordingRepository) MarkGapped(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
) error {
	repo.gaps = append(repo.gaps, recordedGap{
		runID:    runID,
		sequence: sequence,
		encoding: encoding,
		detail:   detail,
	})

	return nil
}

func (repo *recordingRepository) MarkCorrupt(
	runID hindsight.RunID,
	sequence hindsight.CaptureSequence,
	encoding string,
	detail string,
) error {
	return repo.MarkGapped(runID, sequence, encoding, detail)
}

func (repo *recordingRepository) Close() error { return nil }

func BenchmarkWriterCapture(b *testing.B) {
	repository, err := NewSQLite(b.TempDir() + "/capture.sqlite")

	if err != nil {
		b.Fatal(err)
	}

	sequencer, err := hindsight.NewSequencer("benchmark-run")

	if err != nil {
		b.Fatal(err)
	}

	writer, err := NewWriter(repository, sequencer, 4096, 256)

	if err != nil {
		b.Fatal(err)
	}

	payload := []byte(`{"channel":"ticker","type":"update","data":[]}`)
	ref := hindsight.StreamRef{Stream: "public:ticker", Epoch: 1, Sequence: 1}
	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := writer.Capture("ticker", "public", payload, time.Now(), ref); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()

	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}

	if err := repository.Close(); err != nil {
		b.Fatal(err)
	}
}
