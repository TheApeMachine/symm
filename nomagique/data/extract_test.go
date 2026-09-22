package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* TestExtractWrite verifies typed projection and absence across evaluations. */
func TestExtractWrite(t *testing.T) {
	Convey("Given a structured capture document", t, func() {
		ctx := context.Background()
		client := data.Extract_ServerToClient(data.NewExtract(ctx))
		defer client.Release()
		for _, fixture := range []struct {
			path, encoding, expected string
			found                    bool
		}{
			{"cursor", "json", `{"sequence":18446744073709551615}`, true},
			{"symbol", "text", "BTC/USD", true},
			{"truth", "json", "", false},
			{"cursor.sequence", "json", "18446744073709551615", true},
		} {
			So(client.Write(ctx, func(args data.Extract_write_Params) error {
				if err := args.SetPath(fixture.path); err != nil {
					return err
				}
				if err := args.SetEncoding(fixture.encoding); err != nil {
					return err
				}
				return args.SetData([]byte(`{"cursor":{"sequence":18446744073709551615},"symbol":"BTC/USD"}`))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Found(), ShouldEqual, fixture.found)
			if !fixture.found {
				So(result.Which(), ShouldEqual, data.Extracted_Which_missing)
				release()
				continue
			}
			if fixture.encoding == "json" {
				raw, err := result.Json()
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual, fixture.expected)
			}
			if fixture.encoding == "text" {
				value, err := result.Text()
				So(err, ShouldBeNil)
				So(value, ShouldEqual, fixture.expected)
			}
			release()
		}
	})
}

/* BenchmarkExtractWrite measures exact structured projection through Cap'n Proto. */
func BenchmarkExtractWrite(b *testing.B) {
	ctx := context.Background()
	client := data.Extract_ServerToClient(data.NewExtract(ctx))
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(args data.Extract_write_Params) error {
			if err := args.SetPath("cursor"); err != nil {
				return err
			}
			if err := args.SetEncoding("json"); err != nil {
				return err
			}
			return args.SetData([]byte(`{"cursor":{"sequence":18446744073709551615,"record":1},"symbol":"BTC/USD"}`))
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
