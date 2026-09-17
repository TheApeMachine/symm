package tables_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/tablestest"
)

// competingCatalog advances the real table immediately before the writer's
// commit. Requirements are checked against that new head, as REST does for 409.
type competingCatalog struct {
	catalog.Catalog
	beforeCommit func()
	commitErr    error
	attempts     int
	conflicts    int
}

func (catalog *competingCatalog) LoadTable(ctx context.Context, identifier table.Identifier) (*table.Table, error) {
	loaded, err := catalog.Catalog.LoadTable(ctx, identifier)

	if err != nil {
		return nil, err
	}

	return table.New(identifier, loaded.Metadata(), loaded.MetadataLocation(), loaded.FS, catalog), nil
}

func (catalog *competingCatalog) CommitTable(ctx context.Context, identifier table.Identifier, requirements []table.Requirement, updates []table.Update) (table.Metadata, string, error) {
	catalog.attempts++

	if catalog.beforeCommit != nil {
		advance := catalog.beforeCommit
		catalog.beforeCommit = nil
		advance()
	}

	if catalog.commitErr != nil {
		return nil, "", catalog.commitErr
	}

	current, err := catalog.Catalog.LoadTable(ctx, identifier)

	if err != nil {
		return nil, "", err
	}

	for _, requirement := range requirements {
		if err := requirement.Validate(current.Metadata()); err != nil {
			catalog.conflicts++
			return nil, "", fmt.Errorf("%w: %w", table.ErrCommitFailed, err)
		}
	}

	return catalog.Catalog.CommitTable(ctx, identifier, requirements, updates)
}

func TestWriter_CommitReady(t *testing.T) {
	Convey("A writer uses Iceberg's conflict recovery without losing concurrent appends", t, func() {
		savedConfig := system.Cfg
		system.Cfg = &system.Config{Storage: &system.Storage{Iceberg: &system.Iceberg{CommitRetries: 4}}}
		Reset(func() { system.Cfg = savedConfig })
		underlying := tablestest.Underlying(t)
		peerCatalog := tables.Wrap(underlying)
		So(peerCatalog.Ensure(t.Context()), ShouldBeNil)
		adapter := &competingCatalog{Catalog: underlying}
		writer := tables.NewWriter(tables.Wrap(adapter), 100)
		peer := tables.NewWriter(peerCatalog, 100)
		add := func(target *tables.Writer, sequence int64) {
			measurement := data.NewMeasurement(
				"hawkes", map[string]data.Metric[float64]{"intensity": {Raw: float64(sequence)}},
			)
			measurement.Label = "BTC/USD"
			measurement.At = time.Unix(sequence, 0)
			measurement.SeqIdx = sequence
			target.Add("hawkes", measurement)
		}
		add(peer, 1)
		So(peer.CommitReady(t.Context(), true), ShouldBeNil)
		add(writer, 3)

		Convey("A changed branch head refreshes and retains all rows exactly once", func() {
			adapter.beforeCommit = func() {
				add(peer, 2)
				So(peer.CommitReady(t.Context(), true), ShouldBeNil)
			}
			So(writer.CommitReady(t.Context(), true), ShouldBeNil)
			So(adapter.conflicts, ShouldEqual, 1)
			So(adapter.attempts, ShouldEqual, 2)
			So(writer.Pending(), ShouldEqual, 0)
			sequences := map[int64]int{}

			for measurement := range peerCatalog.Scan(t.Context(), tables.Measurements, 100, nil, 0) {
				sequences[measurement.SeqIdx]++
			}

			So(sequences, ShouldResemble, map[int64]int{1: 1, 2: 1, 3: 1})
		})

		Convey("An unknown commit outcome returns immediately and retains pending rows", func() {
			adapter.commitErr = errors.New("gateway timeout: commit outcome unknown")
			So(writer.CommitReady(t.Context(), true), ShouldNotBeNil)
			So(adapter.attempts, ShouldEqual, 1)
			So(writer.Pending(), ShouldEqual, 1)
		})
	})
}

func TestWriterCommitReady(t *testing.T) {
	Convey("Outcome references wait for all tape families to commit", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog, 1)
		measurement := data.NewMeasurement[float64]("trade", nil)
		measurement.SeqIdx = 1
		writer.Add("trade", measurement)
		// Exceed the retired excursion-only batch threshold while the tape is small.
		for index := 0; index < 12; index++ {
			writer.AddExcursion(tables.ExcursionRecord{ID: fmt.Sprint(index), AnchorTick: 1, ExitTick: 2})
		}
		So(writer.CommitReady(t.Context(), false), ShouldBeNil)
		records, err := catalog.Excursions(t.Context(), 1, nil)
		So(err, ShouldBeNil)
		So(records, ShouldBeEmpty)
		So(writer.CommitReady(t.Context(), true), ShouldBeNil)
		records, err = catalog.Excursions(t.Context(), 1, nil)
		So(err, ShouldBeNil)
		So(len(records), ShouldEqual, 12)
		var count int
		for range catalog.Scan(t.Context(), tables.SpotTrade, 1, nil, 0) {
			count++
		}
		So(count, ShouldEqual, 1)
	})
}
