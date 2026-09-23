package arithmetic

import (
	"bytes"
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func decimalQuotient(a, b string, places uint64) (string, error) {
	client := DecimalQuotient_ServerToClient(NewDecimalQuotient(context.Background()))
	defer client.Release()
	out, err := decimalResult(capnp.Client(client), func(ctx context.Context) error {
		return client.Write(ctx, func(params DecimalQuotient_write_Params) error {
			params.SetPlaces(places)
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

func TestDecimalQuotientWrite(t *testing.T) {
	Convey("Given a budget that must also pay a fee", t, func() {
		Convey("Then the quotient is rounded down to the declared places", func() {
			notional, err := decimalQuotient("200", "1.004", 5)
			So(err, ShouldBeNil)
			So(notional, ShouldEqual, "199.20318")
		})

		Convey("Then a zero divisor is an error", func() {
			_, err := decimalQuotient("1", "0", 2)
			So(err, ShouldNotBeNil)
		})
	})
}
