package data_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTree(t *testing.T) {
	ctx := context.Background()

	Convey("Given token paths and the actions they led to", t, func() {
		client := data.Tree_ServerToClient(data.NewTree())
		defer client.Release()

		So(client.Write(ctx, func(params data.Tree_write_Params) error {
			if err := params.SetSeparator("/"); err != nil {
				return err
			}

			return params.SetItems([]byte(`[{"path":"1/2","label":"ENTER"},{"path":"1/2","label":"WAIT"},{"path":"1/3,4","label":"ENTER"},{"path":"5","label":"WAIT"}]`))
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldBeNil)

		future, release := client.Done(ctx, nil)
		defer release()

		results, err := future.Struct()
		So(err, ShouldBeNil)
		So(results.Which(), ShouldEqual, data.Grown_Which_grown)

		Convey("Each node counts the paths through it, and endings become candidates", func() {
			payload, err := results.Grown().Root()
			So(err, ShouldBeNil)

			var root struct {
				Probability float64
				Children    []struct {
					Prefix      string
					Probability float64
					Children    []struct {
						Prefix string
						Tokens []string
						IsEnd  bool
						Label  string
						Share  float64
						Visits int
					}
				}
			}
			So(json.Unmarshal(payload, &root), ShouldBeNil)
			So(root.Probability, ShouldEqual, 1)
			So(root.Children[0].Prefix, ShouldEqual, "1")
			So(root.Children[0].Probability, ShouldEqual, 0.75)
			So(root.Children[0].Children[1].Tokens, ShouldResemble, []string{"3", "4"})
			So(root.Children[0].Children[0].IsEnd, ShouldBeTrue)

			// A node reads as what was recorded there: both labels ended at
			// 1/2 once each, and the tie goes to the first in order.
			So(root.Children[0].Children[0].Label, ShouldEqual, "ENTER")
			So(root.Children[0].Children[0].Share, ShouldEqual, 0.5)
			So(root.Children[0].Children[0].Visits, ShouldEqual, 2)
			So(root.Children[0].Children[1].Label, ShouldEqual, "ENTER")
			So(root.Children[0].Children[1].Share, ShouldEqual, 1)

			payload, err = results.Grown().Candidates()
			So(err, ShouldBeNil)

			var candidates []struct {
				Rank        int
				Action      string
				Prefix      string
				Probability float64
			}
			So(json.Unmarshal(payload, &candidates), ShouldBeNil)
			So(candidates, ShouldHaveLength, 4)
			So(candidates[0].Probability, ShouldEqual, 1)
			So(candidates[0].Rank, ShouldEqual, 1)

			payload, err = results.Grown().Branches()
			So(err, ShouldBeNil)

			var branches []struct {
				Signature  string
				Depth      int
				Visits     int
				Confidence float64
				Policy     string
			}
			So(json.Unmarshal(payload, &branches), ShouldBeNil)
			So(branches, ShouldHaveLength, 3)
			So(branches[0].Signature, ShouldEqual, "1/2")
			So(branches[0].Depth, ShouldEqual, 2)
			So(branches[0].Visits, ShouldEqual, 2)
			So(branches[0].Confidence, ShouldEqual, 0.5)
		})
	})
}
