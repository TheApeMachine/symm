package tables_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_Ensure(t *testing.T) {
	Convey("Given a mock SeaweedFS endpoint", t, func() {
		var tableBucketCreated atomic.Int32
		var s3BucketCreated atomic.Int32

		savedTransport := http.DefaultTransport
		savedCfg := system.Cfg

		Reset(func() {
			system.Cfg = savedCfg
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

		system.Cfg = &system.Config{
			Storage: &system.Storage{
				Iceberg: &system.Iceberg{
					Warehouse: "s3://symmtables/",
				},
				S3: &system.S3{
					Endpoint: "http://mock-seaweedfs",
					Bucket:   "symm",
				},
			},
		}

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

