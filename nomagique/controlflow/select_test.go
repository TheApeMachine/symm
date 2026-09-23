package controlflow_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/controlflow"
)

func TestSelectWrite(t *testing.T) {
	Convey("Given explicit branches, including a deliberately empty branch", t, func() {
		client := controlflow.Select_ServerToClient(controlflow.NewSelect())
		defer client.Release()

		for _, selection := range []struct {
			test              bool
			yes, no, expected string
		}{
			{true, "chosen", "other", "chosen"},
			{false, "other", "chosen", "chosen"},
			{true, "", "must not substitute", ""},
			{false, "must not substitute", "", ""},
		} {
			So(client.Write(context.Background(), func(params controlflow.Select_write_Params) error {
				params.SetTest(selection.test)

				yes, err := params.NewYes(1)
				if err != nil {
					return err
				}
				if err := yes.Set(0, []byte(selection.yes)); err != nil {
					return err
				}

				no, err := params.NewNo(1)
				if err != nil {
					return err
				}
				return no.Set(0, []byte(selection.no))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			if selection.expected == "" {
				So(result.Which(), ShouldEqual, controlflow.Selection_Which_absent)
				release()
				continue
			}
			So(result.Which(), ShouldEqual, controlflow.Selection_Which_out)
			value, err := result.Out()
			So(err, ShouldBeNil)
			So(string(value), ShouldEqual, selection.expected)
			release()
		}
	})
}
