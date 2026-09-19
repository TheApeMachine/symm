package tables_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apache/iceberg-go/table"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_Ensure(t *testing.T) {
	Convey("Configured commit retries also apply to tables created before that setting", t, func() {
		catalog := tablestest.New(t)
		catalog.SetStorageConfig(&tables.StorageConfig{
			Iceberg: tables.IcebergConfig{CommitRetries: 4},
		})
		So(catalog.Ensure(t.Context()), ShouldBeNil)

		for _, name := range []string{tables.SpotTicker, tables.SpotTrade, tables.SpotLevel3, tables.Measurements, tables.Runs, tables.Excursions} {
			loaded, err := catalog.Load(t.Context(), name)
			So(err, ShouldBeNil)
			So(loaded.Properties()[table.CommitNumRetriesKey], ShouldEqual, "4")
			location := loaded.MetadataLocation()
			So(catalog.Ensure(t.Context()), ShouldBeNil)
			loaded, err = catalog.Load(t.Context(), name)
			So(err, ShouldBeNil)
			So(loaded.MetadataLocation(), ShouldEqual, location)
		}
	})

	Convey("Given a mock SeaweedFS endpoint", t, func() {
		var tableBucketCreated atomic.Int32
		var s3BucketCreated atomic.Int32

		savedTransport := http.DefaultTransport

		Reset(func() {
			http.DefaultTransport = savedTransport
		})

		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()

			if req.Method == http.MethodPost && req.Header.Get("X-Amz-Target") == "S3Tables.CreateTableBucket" {
				tableBucketCreated.Add(1)
				rec.WriteHeader(http.StatusOK)
				io.WriteString(rec, `{"arn":"arn:aws:s3tables:us-east-1:000000000000:bucket/symmtables"}`)
				return rec.Result(), nil
			}

			if req.Method == http.MethodPut && req.URL.Path == "/symm" {
				s3BucketCreated.Add(1)
				rec.WriteHeader(http.StatusOK)
				return rec.Result(), nil
			}

			rec.WriteHeader(http.StatusNotFound)
			return rec.Result(), nil
		})

		catalog := tablestest.New(t)
		ctx := context.Background()

		catalog.SetStorageConfig(&tables.StorageConfig{
			Iceberg: tables.IcebergConfig{
				Warehouse: "s3://symmtables/",
			},
			S3: tables.S3Config{
				Endpoint: "http://mock-seaweedfs",
				Bucket:   "symm",
			},
		})

		Convey("Ensure creates the missing table bucket and S3 bucket", func() {
			err := catalog.Ensure(ctx)
			So(err, ShouldBeNil)
			So(tableBucketCreated.Load(), ShouldEqual, 1)
			So(s3BucketCreated.Load(), ShouldEqual, 1)

			Convey("Subsequent Ensure calls handle existing buckets gracefully", func() {
				err := catalog.Ensure(ctx)
				So(err, ShouldBeNil)
			})
		})
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestCatalog_RunsDeduplication(t *testing.T) {
	Convey("Given an Iceberg catalog with duplicate run records for the same epoch", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		at := time.Date(2026, 9, 14, 23, 0, 0, 0, time.UTC)
		epoch := int64(1726348800000000000)

		err := catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: at,
			BuildID:   "symm-live",
			Status:    "ACTIVE",
		})
		So(err, ShouldBeNil)

		err = catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: at.Add(time.Second),
			BuildID:   "symm",
			Status:    "ACTIVE",
		})
		So(err, ShouldBeNil)

		runs, err := catalog.Runs(ctx)
		So(err, ShouldBeNil)
		So(len(runs), ShouldEqual, 1)
		So(runs[0].Epoch, ShouldEqual, epoch)
	})
}

func TestCatalog_ExcursionsRoundtrip(t *testing.T) {
	Convey("Given an Iceberg catalog and writer", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		epoch := int64(2000)

		writer := tables.NewWriter(catalog, epoch)

		record := tables.ExcursionRecord{
			Epoch:              epoch,
			ID:                 "2000:BTC/USD:10",
			Symbol:             "BTC/USD",
			Direction:          "upward",
			ClearsFriction:     true,
			PrecursorStartTick: 1,
			AnchorTick:         10,
			ExtremumTick:       20,
			ExitTick:           28,
			PostEndTick:        35,
			EntryPrice:         50000.0,
			ExtremumPrice:      52500.0,
			ExitPrice:          52000.0,
			PositionSize:       40.0,
			Fee:                0.20,
			Profit:             1.40,
			ProfitFraction:     0.035,
			GrossExcursion:     0.05,
			ObservationCount:   25,
			Status:             "profitable",
		}

		writer.AddExcursion(record)
		So(writer.CommitReady(ctx, true), ShouldBeNil)

		Convey("Excursions queries the stored excursion records for the epoch", func() {
			excursions, err := catalog.Excursions(ctx, epoch, nil)
			So(err, ShouldBeNil)
			So(len(excursions), ShouldEqual, 1)
			read := excursions[0]
			So(read.Symbol, ShouldEqual, "BTC/USD")
			So(read.Direction, ShouldEqual, "upward")
			So(read.ClearsFriction, ShouldBeTrue)
			So(read.Profit, ShouldEqual, 1.40)
			So(read.Status, ShouldEqual, "profitable")
		})
	})
}

func BenchmarkCatalog_Ensure(b *testing.B) {
	catalog := tablestest.New(b)
	catalog.SetStorageConfig(&tables.StorageConfig{
		Iceberg: tables.IcebergConfig{CommitRetries: 4},
	})
	b.ReportAllocs()
	

	for b.Loop() {
		if err := catalog.Ensure(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}
