package tables

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
	. "github.com/smartystreets/goconvey/convey"
)

/* TestIcebergScanWrite requires explicitly supplied file I/O configuration. */
func TestIcebergScanWrite(t *testing.T) {
	Convey("Given valid metadata for an empty local snapshot", t, func() {
		metadata, err := table.NewMetadata(
			iceberg.NewSchema(0, iceberg.NestedField{ID: 1, Name: "value", Type: iceberg.PrimitiveTypes.Int64, Required: true}),
			nil, table.UnsortedSortOrder, "file://"+t.TempDir(), nil,
		)
		So(err, ShouldBeNil)
		encoded, err := json.Marshal(metadata)
		So(err, ShouldBeNil)
		for _, fixture := range []struct {
			name, properties string
			valid            bool
		}{
			{"absent properties", "", false},
			{"null properties", "null", false},
			{"malformed properties", "{", false},
			{"wrong property shape", "[]", false},
			{"explicit empty local filesystem properties", "{}", true},
		} {
			Convey(fixture.name, func() {
				ctx := context.Background()
				client := IcebergScan_ServerToClient(NewIcebergScan())
				defer client.Release()
				So(client.Write(ctx, func(args IcebergScan_write_Params) error {
					metadata, err := args.NewMetadata(1)
					if err != nil {
						return err
					}
					if err := metadata.Set(0, encoded); err != nil {
						return err
					}
					return args.SetProperties([]byte(fixture.properties))
				}), ShouldBeNil)
				err := client.WaitStreaming()
				if !fixture.valid {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "properties")
					return
				}
				So(err, ShouldBeNil)
				future, release := client.Done(ctx, nil)
				defer release()
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Exhausted(), ShouldBeTrue)
			})
		}
	})
}
