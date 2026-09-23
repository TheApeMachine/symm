package arithmetic

import (
	"bytes"
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func decimalAdd(a, b string) (string, error) {
	client := DecimalAdd_ServerToClient(NewDecimalAdd(context.Background()))
	defer client.Release()
	out, err := decimalResult(capnp.Client(client), func(ctx context.Context) error {
		return client.Write(ctx, func(params DecimalAdd_write_Params) error {
			if err := params.SetA([]byte(a)); err != nil {
				return err
			}
			return params.SetB([]byte(b))
		})
	}, func(ctx context.Context) ([]byte, error) {
		future, release := client.Done(ctx, nil)
		defer release()
		results, err := future.Struct()
		if err != nil {
			return nil, err
		}
		out, err := results.Out()
		return bytes.Clone(out), err
	})
	return string(out), err
}

func TestDecimalAddWrite(t *testing.T) {
	Convey("Given two exchange decimals", t, func() {
		Convey("Then the sum keeps every digit either had", func() {
			sum, err := decimalAdd(`"800.0009912"`, "194.4745776")
			So(err, ShouldBeNil)
			So(sum, ShouldEqual, "994.4755688")
		})

		Convey("Then exponent notation is read as the exchange meant it", func() {
			sum, err := decimalAdd("1e-08", "0.00000001")
			So(err, ShouldBeNil)
			So(sum, ShouldEqual, "0.00000002")
		})

		Convey("Then a missing operand is an error, not zero", func() {
			_, err := decimalAdd("", "1")
			So(err, ShouldNotBeNil)
		})
	})
}
