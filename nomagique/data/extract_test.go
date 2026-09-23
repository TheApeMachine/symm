package data_test

import (
	capnp "capnproto.org/go/capnp/v3"
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
			{"cursor", "json-text", `{"sequence":18446744073709551615}`, true},
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
			if fixture.encoding == "text" || fixture.encoding == "json-text" {
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

func TestExtractWriteUnsigned(t *testing.T) {
	Convey("Given exact integer projection", t, func() {
		ctx := context.Background()
		for _, fixture := range []struct {
			payload      string
			expected     uint64
			valid, found bool
		}{
			{`{"index":0}`, 0, true, true},
			{`{"index":9007199254740993}`, 9007199254740993, true, true},
			{`{"index":18446744073709551615}`, ^uint64(0), true, true},
			{`{}`, 0, true, false},
			{`{"index":-1}`, 0, false, false},
			{`{"index":1.5}`, 0, false, false},
			{`{"index":"1"}`, 0, false, false},
			{`{"index":null}`, 0, false, false},
			{`{"index":18446744073709551616}`, 0, false, false},
		} {
			client := data.Extract_ServerToClient(data.NewExtract(ctx))
			So(client.Write(ctx, func(args data.Extract_write_Params) error {
				if err := args.SetPath("index"); err != nil {
					return err
				}
				if err := args.SetEncoding("uint64"); err != nil {
					return err
				}
				return args.SetData([]byte(fixture.payload))
			}), ShouldBeNil)
			err := client.WaitStreaming()
			if !fixture.valid {
				So(err, ShouldNotBeNil)
				client.Release()
				continue
			}
			So(err, ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Found(), ShouldEqual, fixture.found)
			if fixture.found {
				So(result.Unsigned() == fixture.expected, ShouldBeTrue)
			}
			if !fixture.found {
				So(result.Which(), ShouldEqual, data.Extracted_Which_missing)
			}
			release()
			client.Release()
		}
	})
}

func TestExtractWriteEncoding(t *testing.T) {
	Convey("Given a non-JSON payload on a numeric JSON projection", t, func() {
		ctx := context.Background()
		client := data.Extract_ServerToClient(data.NewExtract(ctx))
		defer client.Release()
		// This is a real Cap'n Proto message. JSON projection must not auto-detect it.
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		measurement, err := data.NewRootMeasurement(segment)
		So(err, ShouldBeNil)
		metrics, err := measurement.NewMetrics(1)
		So(err, ShouldBeNil)
		metrics.At(0).SetRaw(123)
		payload, err := message.Marshal()
		So(err, ShouldBeNil)
		So(client.Write(ctx, func(args data.Extract_write_Params) error {
			if err := args.SetPath("0"); err != nil {
				return err
			}
			if err := args.SetEncoding("number"); err != nil {
				return err
			}
			return args.SetData(payload)
		}), ShouldBeNil)
		err = client.WaitStreaming()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "not a structure")
	})
}
