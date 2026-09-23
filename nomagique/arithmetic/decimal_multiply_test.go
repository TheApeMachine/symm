package arithmetic

import (
	"bytes"
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func decimalMultiply(a, b string) (string, error) {
	client := DecimalMultiply_ServerToClient(NewDecimalMultiply(context.Background()))
	defer client.Release()
	out, err := decimalResult(capnp.Client(client), func(ctx context.Context) error {
		return client.Write(ctx, func(params DecimalMultiply_write_Params) error {
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

func TestDecimalMultiplyWrite(t *testing.T) {
	Convey("Given a price and a quantity", t, func() {
		Convey("Then an odd whole product is not rounded, unlike the exchange SDK's Decimal", func() {
			product, err := decimalMultiply("99", "1")
			So(err, ShouldBeNil)
			So(product, ShouldEqual, "99")
		})

		Convey("Then the product keeps the places of both operands", func() {
			product, err := decimalMultiply("101", "0.9822")
			So(err, ShouldBeNil)
			So(product, ShouldEqual, "99.2022")
		})
	})
}
