package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* TestFilterWrite exercises exact capture ordering through the real capability. */
func TestFilterWrite(t *testing.T) {
	Convey("Given one filter reused across capture comparisons", t, func() {
		ctx := context.Background()
		client := data.Filter_ServerToClient(data.NewFilter(ctx))
		defer client.Release()
		evaluate := func(payload, path, reference, operator string, threshold float64) bool {
			So(client.Write(ctx, func(args data.Filter_write_Params) error {
				args.SetThreshold(threshold)
				if err := args.SetPath(path); err != nil {
					return err
				}
				if err := args.SetReferencePath(reference); err != nil {
					return err
				}
				if err := args.SetOperator(operator); err != nil {
					return err
				}
				return args.SetData([]byte(payload))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			if !result.Passed() {
				So(result.Which(), ShouldEqual, data.Filtered_Which_rejected)
				return false
			}
			So(result.Which(), ShouldEqual, data.Filtered_Which_out)
			output, err := result.Out()
			So(err, ShouldBeNil)
			So(string(output), ShouldEqual, payload)
			return true
		}
		Convey("Adjacent sequence numbers above float precision remain distinct", func() {
			payload := `{"cursor":{"sequence":9007199254740993,"record":0},"b":{"sequence":9007199254740992,"record":8}}`
			So(evaluate(payload, "cursor.sequence,cursor.record", "b.sequence,b.record", ">", 0), ShouldBeTrue)
			So(evaluate(payload, "cursor.sequence,cursor.record", "b.sequence,b.record", "==", 0), ShouldBeFalse)
		})
		Convey("Record position breaks ties only within the same capture", func() {
			payload := `{"cursor":{"sequence":18446744073709551615,"record":2},"b":{"sequence":18446744073709551615,"record":3}}`
			So(evaluate(payload, "cursor.sequence,cursor.record", "b.sequence,b.record", "<", 0), ShouldBeTrue)
			So(evaluate(payload, "cursor.sequence,cursor.record", "b.sequence,b.record", ">=", 0), ShouldBeFalse)
		})
		Convey("Missing boundaries cannot pass a comparison", func() {
			So(evaluate(`{"cursor":{"sequence":1,"record":0}}`, "cursor.sequence,cursor.record", "b.sequence,b.record", "<", 0), ShouldBeFalse)
			So(evaluate(`{}`, "a", "", "exists", 0), ShouldBeFalse)
			So(evaluate(`{}`, "a", "", "absent", 0), ShouldBeTrue)
		})
		Convey("Zero replaces the preceding threshold", func() {
			So(evaluate(`{"value":1}`, "value", "", ">", 2), ShouldBeFalse)
			So(evaluate(`{"value":1}`, "value", "", ">", 0), ShouldBeTrue)
		})
	})
}

/* BenchmarkFilterWrite includes Cap'n Proto invocation and exact cursor comparison. */
func BenchmarkFilterWrite(b *testing.B) {
	ctx := context.Background()
	client := data.Filter_ServerToClient(data.NewFilter(ctx))
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(args data.Filter_write_Params) error {
			if err := args.SetPath("cursor.sequence,cursor.record"); err != nil {
				return err
			}
			if err := args.SetReferencePath("b.sequence,b.record"); err != nil {
				return err
			}
			if err := args.SetOperator(">="); err != nil {
				return err
			}
			return args.SetData([]byte(`{"cursor":{"sequence":9007199254740993,"record":0},"b":{"sequence":9007199254740992,"record":8}}`))
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
