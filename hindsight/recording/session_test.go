package recording

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
	"golang.design/x/lockfree/lf"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/store"
)

/*
TestSessionCapture exercises the real production capture →
envelope → semantic → witness → persisted-record chain. One raw Kraken trade
frame yielding two trades is captured through the real Archive store + sequencer
+ writer, parsed by the real ingest path into two envelopes with deterministic
ordinals, advanced through the real category solver, and its artifacts witnessed
and persisted. The test then answers, from persisted identities alone: what
exact exchange bytes, parsed envelope, and processing transition caused a
resulting semantic artifact.
*/
func TestSessionCapture(t *testing.T) {
	Convey("Given the real capture and semantic stack", t, func() {

		runID, err := hindsight.NewRunID(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		So(err, ShouldBeNil)
		writer, engine := newSessionFixture(t, runID)

		Convey("One raw frame yielding two trades is captured, ingested, witnessed, and traversable by identity", func() {
			raw := []byte(`{"channel":"trade","type":"update","data":[
				{"symbol":"XBT/USD","side":"buy","price":"34000.5","qty":0.1,"ord_type":"market","trade_id":1,"timestamp":"2026-01-02T03:04:05Z"},
				{"symbol":"XBT/USD","side":"sell","price":"34001.0","qty":0.2,"ord_type":"market","trade_id":2,"timestamp":"2026-01-02T03:04:05Z"}
			]}`)

			// 1. Capture the raw frame: mint + persist identity.
			captureID, err := writer.Capture("trade", "wss://example", raw, time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})
			So(err, ShouldBeNil)
			So(captureID.Valid(), ShouldBeTrue)

			// 2. Parse via the production ingest path: two envelopes, one origin.
			parsed := kraken.NewTrade(raw)
			envelopes, manifests := websocket.IngestEnvelopes("trade", parsed, captureID)

			So(len(envelopes), ShouldEqual, 2)
			So(len(manifests), ShouldEqual, 2)
			So(envelopes[0].CaptureID, ShouldResemble, captureID)
			So(envelopes[1].CaptureID, ShouldResemble, captureID)
			So(envelopes[0].CaptureOrdinal, ShouldEqual, uint64(0))
			So(envelopes[1].CaptureOrdinal, ShouldEqual, uint64(1))

			// 3. Persist the envelope manifests (the live ingress does this too).
			for _, manifest := range manifests {
				So(writer.WriteManifest(manifest), ShouldBeNil)
			}

			// 4. Advance one envelope through the real category solver to produce
			// a semantic artifact (a Measurement is the category solver's input).
			solver := category.NewSolver(context.Background())

			measurement := data.NewMeasurement[float64](
				"cvd-1", "XBT/USD", "cvd",
				time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
			)
			measurement.PutMetric(data.Metric[float64]{
				Label: "signed_net_fraction_zscore",
				Raw:   1.5,
			})

			envelopes[0].CVD = measurement
			solver.Step(envelopes[0])

			// 5. Witness the semantic artifact (the Measurement) on the first
			// envelope, exactly as the live witnessNode records it.
			ref := hindsight.EnvelopeRef{Origin: captureID, Ordinal: 0}

			writer.record(hindsight.ArtifactWitness{
				Envelope:         ref,
				Boundary:         "after-signals",
				Artifact:         hindsight.ArtifactID{Kind: "measurement", Identity: "cvd-1"},
				ImmediateParents: []hindsight.EnvelopeRef{ref},
			})
			So(writer.Close(), ShouldBeNil)

			// 6. From persisted identities alone, traverse back to the exact
			// exchange bytes. The raw frame is stored alongside its identity.
			stored, found, err := store.Find(context.Background(), engine, captureID.Run.Prefix("captures"), func(frame hindsight.RawFrame) bool { return frame.Identity == captureID })
			So(found, ShouldBeTrue)
			So(err, ShouldBeNil)
			So(string(stored.Payload), ShouldEqual, string(raw))

			// The witness is persisted, keyed by the same origin so a consumer
			// can walk witness → EnvelopeRef → raw frame without any timestamp.
			witnesses, err := store.List[hindsight.ArtifactWitness](context.Background(), engine, runID.Prefix("witnesses"))
			So(witnesses, ShouldHaveLength, 1)
			witness := witnesses[0]
			So(err, ShouldBeNil)
			So(witness.Artifact.Kind, ShouldEqual, "measurement")
			So(witness.Artifact.Identity, ShouldEqual, "cvd-1")
			So(witness.Envelope.Origin, ShouldResemble, captureID)
			So(witness.Envelope.Ordinal, ShouldEqual, uint64(0))
			So(witness.ImmediateParents, ShouldHaveLength, 1)
			So(witness.ImmediateParents[0], ShouldResemble, ref)
		})
	})
}

func newSessionFixture(t *testing.T, run hindsight.RunID) (*Session, *blob.Bucket) {
	t.Helper()
	bucket := memblob.OpenBucket(nil)
	writer, err := NewSession(context.Background(), bucket, hindsight.Run{ID: run, StartedAt: time.Unix(1, 0)}, 8, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close recorder: %v", err)
		}
		if err := bucket.Close(); err != nil {
			t.Errorf("close bucket: %v", err)
		}
	})
	return writer, bucket
}

// delayedBucket exercises the real recorder and blob writer with upload latency
// longer than the flush interval. It retains the exact uploaded bytes.
type delayedBucket struct {
	driver.Bucket
	mutex   sync.Mutex
	objects [][]byte
	started chan struct{}
	release chan struct{}
	err     error
}

func (bucket *delayedBucket) NewTypedWriter(ctx context.Context, key, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	return &delayedWriter{bucket: bucket}, nil
}

func (bucket *delayedBucket) Close() error { return nil }

func (bucket *delayedBucket) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerrors.Unknown
}

type delayedWriter struct {
	bytes.Buffer
	bucket *delayedBucket
}

func (writer *delayedWriter) Close() error {
	if writer.bucket.release != nil {
		select {
		case writer.bucket.started <- struct{}{}:
		default:
		}
		<-writer.bucket.release
	}

	if writer.bucket.err != nil {
		return writer.bucket.err
	}
	time.Sleep(15 * time.Millisecond) // Fixture upload exceeds the 1ms batching interval.
	writer.bucket.mutex.Lock()
	defer writer.bucket.mutex.Unlock()
	writer.bucket.objects = append(writer.bucket.objects, bytes.Clone(writer.Bytes()))
	return nil
}

func TestSessionPersist(t *testing.T) {
	Convey("Given queued records and uploads slower than the flush interval", t, func() {
		synctest.Test(t, func(t *testing.T) {
			storage := &delayedBucket{}
			bucket := blob.NewBucket(storage)
			session := &Session{bucket: bucket, queue: lf.NewQueue[pending](), done: make(chan struct{})}
			for index := range 82 {
				session.queue.Enqueue(pending{key: "learning/run/record.json", data: []byte(fmt.Sprintf("%d", index))})
			}
			session.closed = true
			session.persist(context.Background(), 8, time.Millisecond)
			if len(storage.objects) != 11 {
				t.Fatalf("82 records must use ten full batches and one partial batch; got %d uploads", len(storage.objects))
			}
			records := strings.Split(string(bytes.Join(storage.objects, []byte("\n"))), "\n")
			for index, record := range records {
				if record != fmt.Sprint(index) {
					t.Fatalf("record %d: got %q", index, record)
				}
			}
			if err := bucket.Close(); err != nil {
				t.Fatal(err)
			}
		})
	})
}

func BenchmarkSessionPersist(b *testing.B) {
	bucket := memblob.OpenBucket(nil)
	defer func() {
		if err := bucket.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	payload := bytes.Repeat([]byte("x"), 1024) // 1KiB original transition records.
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		session := &Session{bucket: bucket, queue: lf.NewQueue[pending](), done: make(chan struct{})}
		for range 1024 {
			session.queue.Enqueue(pending{key: "learning/run/record.json", data: payload})
		}
		session.closed = true
		session.persist(context.Background(), 256, time.Second)
	}
}

func TestSessionWriteLearning(t *testing.T) {
	Convey("Given an S3 upload held while producers continue", t, func() {
		synctest.Test(t, func(t *testing.T) {
			storage := &delayedBucket{started: make(chan struct{}, 1), release: make(chan struct{})}
			bucket := blob.NewBucket(storage)
			session := &Session{
				bucket: bucket, run: hindsight.Run{ID: "burst"},
				queue: lf.NewQueue[pending](), wake: make(chan struct{}, 1),
				done: make(chan struct{}), Errors: make(chan error, 1),
			}
			go session.persist(context.Background(), 256, time.Millisecond)
			type event struct {
				Producer int
				Sequence int
				Payload  []byte
			}
			if err := session.WriteLearning(event{Producer: -1}); err != nil {
				t.Fatal(err)
			}
			<-storage.started
			// Four producers exceed the former 8192-record capacity during one upload.
			var producers sync.WaitGroup
			for producer := range 4 {
				producers.Go(func() {
					record := event{Producer: producer, Payload: []byte("original")}
					for sequence := range 4096 {
						record.Sequence = sequence
						if err := session.WriteLearning(record); err != nil {
							t.Error(err)
							return
						}
					}
					copy(record.Payload, "mutated!")
				})
			}
			producers.Wait()
			closed := make(chan error, 1)
			go func() { closed <- session.Close() }()
			synctest.Wait()
			select {
			case err := <-closed:
				t.Fatalf("close returned before the upload completed: %v", err)
			default:
			}
			close(storage.release)
			if err := <-closed; err != nil {
				t.Fatal(err)
			}
			next := make([]int, 4)
			records := strings.Split(string(bytes.Join(storage.objects, []byte("\n"))), "\n")
			if len(records) != 1+4*4096 {
				t.Fatalf("accepted records lost or duplicated: %d", len(records))
			}
			for _, raw := range records[1:] {
				var record event
				if err := json.Unmarshal([]byte(raw), &record); err != nil {
					t.Fatal(err)
				}
				if record.Sequence != next[record.Producer] || string(record.Payload) != "original" {
					t.Fatalf("out of order or mutated record: %+v", record)
				}
				next[record.Producer]++
			}
			if err := session.WriteLearning(event{}); err == nil {
				t.Fatal("closed session accepted a record")
			}
			if err := bucket.Close(); err != nil {
				t.Fatal(err)
			}
		})
	})
	Convey("Given an S3 write failure", t, func() {
		synctest.Test(t, func(t *testing.T) {
			failure := errors.New("fixture S3 failure")
			bucket := blob.NewBucket(&delayedBucket{err: failure})
			session := &Session{
				bucket: bucket, run: hindsight.Run{ID: "failure"},
				queue: lf.NewQueue[pending](), wake: make(chan struct{}, 1),
				done: make(chan struct{}), Errors: make(chan error, 1),
			}
			go session.persist(context.Background(), 1, time.Hour)
			if err := session.WriteLearning("accepted"); err != nil {
				t.Fatal(err)
			}
			if err := <-session.Errors; !errors.Is(err, failure) {
				t.Fatalf("missing upload failure: %v", err)
			}
			if err := session.WriteLearning("rejected"); !errors.Is(err, failure) {
				t.Fatalf("admission did not report upload failure: %v", err)
			}
			if err := session.Close(); !errors.Is(err, failure) {
				t.Fatalf("close did not report upload failure: %v", err)
			}
			if err := bucket.Close(); err != nil {
				t.Fatal(err)
			}
		})
	})
}
