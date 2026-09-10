package tables_test

import (
	"bytes"
	"context"
	"errors"

	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"
	"github.com/spf13/viper"
	"math"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
)

func TestWriterCommit(t *testing.T) {
	if os.Getenv("SYMM_LARGE_PAYLOAD_TEST") != "1" {
		t.Skip("set SYMM_LARGE_PAYLOAD_TEST=1 to exercise the real 2 GiB Arrow offset boundary")
	}

	Convey("One snapshot preserves binary payloads exceeding a signed 32-bit offset in total", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		// Repeating a one-MiB payload crosses the actual Arrow Binary boundary
		// without retaining separate copies in the input fixture.
		payload := bytes.Repeat([]byte("x"), 1<<20)
		count := math.MaxInt32/len(payload) + 2

		for index := range count {
			writer.AddWitness(tables.WitnessRow{
				Run: "large", ArtifactKind: "precursor",
				Envelope: tables.EnvelopeRefRow{Run: "large", Sequence: int64(index + 1)},
				Payload:  payload,
			})
		}
		So(writer.Commit(t.Context()), ShouldBeNil)
		loaded, err := catalog.Load(t.Context(), tables.Witnesses)
		So(err, ShouldBeNil)
		So(len(loaded.Metadata().Snapshots()), ShouldEqual, 1)
		rows, err := catalog.Witnesses(t.Context(), "large", "precursor")
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, count)

		seen := make(map[int64]bool, count)
		for _, row := range rows {
			So(seen[row.Envelope.Sequence], ShouldBeFalse)
			seen[row.Envelope.Sequence] = true
			So(bytes.Equal(row.Payload, payload), ShouldBeTrue)
		}
		after, err := catalog.Load(t.Context(), tables.Witnesses)
		So(err, ShouldBeNil)
		So(after.MetadataLocation(), ShouldEqual, loaded.MetadataLocation())

		Convey("A single unrepresentable payload fails before another snapshot is committed", func() {
			writer.AddWitness(tables.WitnessRow{
				Run: "large", ArtifactKind: "precursor",
				Payload: make([]byte, int64(math.MaxInt32)+1),
			})
			err := writer.Commit(t.Context())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "one payload exceeds Arrow Binary")
			after, err := catalog.Load(t.Context(), tables.Witnesses)
			So(err, ShouldBeNil)
			So(after.MetadataLocation(), ShouldEqual, loaded.MetadataLocation())
		})
	})
}

// competingCatalog keeps real files and catalog transactions, injecting the
// REST catalog's explicit rejected-commit signal at the stale-head boundary.
// SQLite's catalog does not classify its conflict as table.ErrCommitFailed.
type competingCatalog struct {
	catalog.Catalog
	beforeCommit func(context.Context) error
	attempts     int
	afterCommit  error
}

func (competing *competingCatalog) LoadTable(
	ctx context.Context, identifier table.Identifier,
) (*table.Table, error) {
	loaded, err := competing.Catalog.LoadTable(ctx, identifier)

	if err != nil {
		return nil, err
	}

	return table.New(identifier, loaded.Metadata(), loaded.MetadataLocation(), loaded.FS, competing), nil
}

func (competing *competingCatalog) CommitTable(
	ctx context.Context, identifier table.Identifier,
	requirements []table.Requirement, updates []table.Update,
) (table.Metadata, string, error) {
	competing.attempts++

	if competing.beforeCommit != nil {
		if err := competing.beforeCommit(ctx); err != nil {
			return nil, "", err
		}
	}

	metadata, location, err := competing.Catalog.CommitTable(ctx, identifier, requirements, updates)

	if err != nil {
		return nil, "", err
	}

	if competing.afterCommit != nil {
		return nil, "", competing.afterCommit
	}

	return metadata, location, nil
}

type refusingCatalog struct {
	catalog.Catalog
	name string
}

func (refusing *refusingCatalog) LoadTable(
	ctx context.Context, identifier table.Identifier,
) (*table.Table, error) {
	if identifier[len(identifier)-1] == refusing.name {
		return nil, errors.New("catalog refused " + refusing.name)
	}

	return refusing.Catalog.LoadTable(ctx, identifier)
}

func TestWriterCommitChunks(t *testing.T) {
	Convey("Buffered rows become successive snapshots inside the catalog payload bound", t, func() {
		previous := viper.Get("storage.iceberg.append_bytes")
		t.Cleanup(func() { viper.Set("storage.iceberg.append_bytes", previous) })
		viper.Set("storage.iceberg.append_bytes", 4096)
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		payload := bytes.Repeat([]byte("w"), 2000)

		for index := range 5 {
			writer.AddWitness(tables.WitnessRow{
				Run: "chunks", ArtifactKind: "precursor",
				Envelope: tables.EnvelopeRefRow{Run: "chunks", Sequence: int64(index + 1)},
				Payload:  payload,
			})
		}
		So(writer.Commit(t.Context()), ShouldBeNil)
		So(writer.Pending(), ShouldEqual, 0)
		loaded, err := catalog.Load(t.Context(), tables.Witnesses)
		So(err, ShouldBeNil)
		So(len(loaded.Metadata().Snapshots()), ShouldEqual, 3)
		rows, err := catalog.Witnesses(t.Context(), "chunks", "precursor")
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 5)
	})
}

func TestWriterCommitRestores(t *testing.T) {
	Convey("A family that never reached the catalog is still buffered", t, func() {
		underlying := tablestest.Underlying(t)
		peer := tables.Wrap(underlying)
		So(peer.Ensure(t.Context()), ShouldBeNil)
		writer := tables.NewWriter(tables.Wrap(&refusingCatalog{
			Catalog: underlying, name: tables.Witnesses,
		}))
		writer.AddCapture(tables.CaptureRow{
			Run: "restore", Sequence: 1, Payload: []byte("capture"),
		})
		writer.AddWitness(tables.WitnessRow{
			Run: "restore", ArtifactKind: "precursor",
			Envelope: tables.EnvelopeRefRow{Run: "restore", Sequence: 1},
			Payload:  []byte("witness"),
		})
		So(writer.Commit(t.Context()), ShouldNotBeNil)
		So(writer.Pending(), ShouldEqual, 1)
		rows, err := peer.Captures(t.Context(), "restore", 0)
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 1)
		So(string(rows[0].Payload), ShouldEqual, "capture")
		witnesses, err := peer.Witnesses(t.Context(), "restore", "")
		So(err, ShouldBeNil)
		So(len(witnesses), ShouldEqual, 0)
	})
}

func TestWriterAppend(t *testing.T) {
	Convey("An append handles known conflicts without repeating a batch", t, func() {
		previous := viper.Get("storage.iceberg.commit_retries")
		t.Cleanup(func() { viper.Set("storage.iceberg.commit_retries", previous) })
		// Two retries allow the fixture to distinguish success, exhaustion,
		// and an error that must never be retried.
		viper.Set("storage.iceberg.commit_retries", 2)
		underlying := tablestest.Underlying(t)
		peerCatalog := tables.Wrap(underlying)
		So(peerCatalog.Ensure(t.Context()), ShouldBeNil)
		competing := &competingCatalog{Catalog: underlying}
		writer := tables.NewWriter(tables.Wrap(competing))
		writer.AddCapture(tables.CaptureRow{
			Run: "writer", Sequence: 1, Payload: []byte("writer payload"),
		})

		Convey("A peer advances the branch before this writer commits", func() {
			competing.beforeCommit = func(ctx context.Context) error {
				competing.beforeCommit = nil
				peer := tables.NewWriter(peerCatalog)
				peer.AddCapture(tables.CaptureRow{
					Run: "peer", Sequence: 1, Payload: []byte("peer payload"),
				})

				if err := peer.Commit(ctx); err != nil {
					return err
				}

				return table.ErrCommitFailed
			}
			So(writer.Commit(t.Context()), ShouldBeNil)
			So(competing.attempts, ShouldEqual, 2)

			for _, run := range []string{"writer", "peer"} {
				rows, err := peerCatalog.Captures(t.Context(), run, 0)
				So(err, ShouldBeNil)
				So(len(rows), ShouldEqual, 1)
				So(string(rows[0].Payload), ShouldEqual, run+" payload")
			}
		})

		Convey("A non-conflict error is returned after one attempt", func() {
			failure := errors.New("catalog outcome unknown")
			competing.beforeCommit = func(context.Context) error { return failure }
			So(writer.Commit(t.Context()), ShouldNotBeNil)
			So(competing.attempts, ShouldEqual, 1)
		})

		Convey("An accepted commit with a lost response is not appended again", func() {
			competing.afterCommit = errors.New("response lost after acceptance")
			So(writer.Commit(t.Context()), ShouldNotBeNil)
			So(competing.attempts, ShouldEqual, 1)
			rows, err := peerCatalog.Captures(t.Context(), "writer", 0)
			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
		})

		Convey("Repeated rejections stop at the configured budget", func() {
			competing.beforeCommit = func(context.Context) error { return table.ErrCommitFailed }
			So(writer.Commit(t.Context()), ShouldNotBeNil)
			So(competing.attempts, ShouldEqual, 3)
			rows, err := peerCatalog.Captures(t.Context(), "writer", 0)
			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 0)
		})
	})
}

func BenchmarkWriterCommit(b *testing.B) {
	catalog := tablestest.New(b)
	writer := tables.NewWriter(catalog)
	// One raw book-shaped payload per append measures the actual file and
	// metadata commit path; no catalog calls or data writes are substituted.
	payload := bytes.Repeat([]byte("captured order book"), 4096)
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		writer.AddCapture(tables.CaptureRow{
			Run: "benchmark", Sequence: int64(index + 1), Payload: payload,
		})

		if err := writer.Commit(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}
