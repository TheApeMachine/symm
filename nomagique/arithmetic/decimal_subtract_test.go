package arithmetic

import (
	"bytes"
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func decimalSubtract(a, b string) (string, error) {
	client := DecimalSubtract_ServerToClient(NewDecimalSubtract(context.Background()))
	defer client.Release()
	out, err := decimalResult(capnp.Client(client), func(ctx context.Context) error {
		return client.Write(ctx, func(params DecimalSubtract_write_Params) error {
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

func TestDecimalSubtractWrite(t *testing.T) {
	Convey("Given a round trip's proceeds and basis", t, func() {
		pnl, err := decimalSubtract("194.4745776", "199.9990088")
		So(err, ShouldBeNil)
		So(pnl, ShouldEqual, "-5.5244312")
	})
}
