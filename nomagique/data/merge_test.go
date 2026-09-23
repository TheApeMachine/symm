package data_test

import (
	"context"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
)

/* TestMergeWrite verifies explicit precedence and rejects absent operands. */
func TestMergeWrite(t *testing.T) {
	Convey("Given graph-supplied object operands", t, func() {
		for _, fixture := range []struct {
			name, base, overlay, want string
			valid                     bool
		}{
			{"overlay wins", `{"uri":"old","sequence":9007199254740993}`, `{"uri":"new"}`, `{"sequence":9007199254740993,"uri":"new"}`, true},
			{"empty object is an explicit operand", `{}`, `{"uri":"new"}`, `{"uri":"new"}`, true},
			{"missing base is not an empty object", ``, `{}`, ``, false},
			{"missing overlay is not an empty object", `{}`, ``, ``, false},
			{"null is not an object", `{}`, `null`, ``, false},
			{"arrays are not objects", `[]`, `{}`, ``, false},
		} {
			Convey(fixture.name, func() {
				ctx := context.Background()
				client := data.Merge_ServerToClient(data.NewMerge())
				defer client.Release()
				So(client.Write(ctx, func(args data.Merge_write_Params) error {
					if err := args.SetBase([]byte(fixture.base)); err != nil {
						return err
					}
					return args.SetOverlay([]byte(fixture.overlay))
				}), ShouldBeNil)
				err := client.WaitStreaming()
				if !fixture.valid {
					So(err, ShouldNotBeNil)
					return
				}
				So(err, ShouldBeNil)
				future, release := client.Done(ctx, nil)
				defer release()
				result, err := future.Struct()
				So(err, ShouldBeNil)
				raw, err := result.Out()
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual, fixture.want)
			})
		}
	})
}

/* BenchmarkMergeWrite measures configuration composition through Cap'n Proto. */
func BenchmarkMergeWrite(b *testing.B) {
	ctx := context.Background()
	client := data.Merge_ServerToClient(data.NewMerge())
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(args data.Merge_write_Params) error {
			if err := args.SetBase([]byte(`{"uri":"https://catalog.test","sequence":9007199254740993}`)); err != nil {
				return err
			}
			return args.SetOverlay([]byte(`{"warehouse":"file:///archive"}`))
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
