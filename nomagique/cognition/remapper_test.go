package cognition_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestRemapper(t *testing.T) {
	Convey("Given a Remapper capability", t, func() {
		ctx := context.Background()
		server := cognition.NewRemapper()
		So(server, ShouldNotBeNil)

		client := cognition.Remapper_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("Absent evidence structurally remains unsettled with no tokens", func() {
			err := client.Write(ctx, func(params cognition.Remapper_write_Params) error {
				ids, err := params.NewIds(4)
				So(err, ShouldBeNil)
				So(ids.Set(0, "metric_a"), ShouldBeNil)
				So(ids.Set(1, "metric_b"), ShouldBeNil)
				So(ids.Set(2, "metric_c"), ShouldBeNil)
				So(ids.Set(3, "metric_d"), ShouldBeNil)

				acts, err := params.NewActivations(4)
				So(err, ShouldBeNil)
				acts.Set(0, 1.5)
				acts.Set(1, 0.2)
				acts.Set(2, 2.0)
				acts.Set(3, 0.1)

				auths, err := params.NewAuthorities(4)
				So(err, ShouldBeNil)
				auths.Set(0, 10.0)
				auths.Set(1, 1.0)
				auths.Set(2, 8.0)
				auths.Set(3, 0.5)

				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			So(results.Settled(), ShouldBeFalse)
			So(results.Revision(), ShouldEqual, 0)

			tokens, err := results.Tokens()
			So(err, ShouldBeNil)
			So(tokens.Len(), ShouldEqual, 0)
		})

		Convey("Missing metric IDs returns validation error", func() {
			err := client.Write(ctx, func(params cognition.Remapper_write_Params) error {
				acts, _ := params.NewActivations(2)
				acts.Set(0, 1.0)
				acts.Set(1, 1.0)
				auths, _ := params.NewAuthorities(2)
				auths.Set(0, 1.0)
				auths.Set(1, 1.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})

		Convey("Malformed evidence JSON returns error", func() {
			err := client.Write(ctx, func(params cognition.Remapper_write_Params) error {
				ids, _ := params.NewIds(2)
				ids.Set(0, "metric_a")
				ids.Set(1, "metric_b")
				acts, _ := params.NewActivations(2)
				acts.Set(0, 1.0)
				acts.Set(1, 1.0)
				auths, _ := params.NewAuthorities(2)
				auths.Set(0, 1.0)
				auths.Set(1, 1.0)
				params.SetEvidence([]byte("not valid json"))
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})

		Convey("Evidence drives strict descent settling and emits tokens and cursor", func() {
			evidenceJSON := []byte(`{
				"metric_a:metric_d": {"sympathy": -50.0, "magnitude": 5.0}
			}`)
			cursorJSON := []byte(`{"sequence": 42, "record": 100}`)

			// First write: triggers layout reorganization
			err := client.Write(ctx, func(params cognition.Remapper_write_Params) error {
				params.SetReset(true)
				ids, _ := params.NewIds(4)
				ids.Set(0, "metric_a")
				ids.Set(1, "metric_b")
				ids.Set(2, "metric_c")
				ids.Set(3, "metric_d")

				acts, _ := params.NewActivations(4)
				acts.Set(0, 2.0)
				acts.Set(1, 0.5)
				acts.Set(2, 1.8)
				acts.Set(3, 0.2)

				auths, _ := params.NewAuthorities(4)
				auths.Set(0, 10.0)
				auths.Set(1, 2.0)
				auths.Set(2, 9.0)
				auths.Set(3, 1.0)

				params.SetEvidence(evidenceJSON)
				params.SetCursor(cursorJSON)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future1, release1 := client.Done(ctx, nil)
			defer release1()
			res1, err := future1.Struct()
			So(err, ShouldBeNil)

			// Next write with the same evidence: reaches minimum, no further swap possible
			err = client.Write(ctx, func(params cognition.Remapper_write_Params) error {
				ids, _ := params.NewIds(4)
				ids.Set(0, "metric_a")
				ids.Set(1, "metric_b")
				ids.Set(2, "metric_c")
				ids.Set(3, "metric_d")

				acts, _ := params.NewActivations(4)
				acts.Set(0, 2.0)
				acts.Set(1, 0.5)
				acts.Set(2, 1.8)
				acts.Set(3, 0.2)

				auths, _ := params.NewAuthorities(4)
				auths.Set(0, 10.0)
				auths.Set(1, 2.0)
				auths.Set(2, 9.0)
				auths.Set(3, 1.0)

				params.SetEvidence(evidenceJSON)
				params.SetCursor(cursorJSON)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future2, release2 := client.Done(ctx, nil)
			defer release2()
			res2, err := future2.Struct()
			So(err, ShouldBeNil)

			So(res2.Settled(), ShouldBeTrue)
			So(res2.Revision(), ShouldBeGreaterThanOrEqualTo, 1)

			tokens, err := res2.Tokens()
			So(err, ShouldBeNil)
			So(tokens.Len(), ShouldBeGreaterThan, 0)

			outBytes, err := res2.Out()
			So(err, ShouldBeNil)
			So(len(outBytes), ShouldBeGreaterThan, 0)

			var outDoc map[string]any
			So(json.Unmarshal(outBytes, &outDoc), ShouldBeNil)
			So(outDoc["settled"], ShouldEqual, true)
			So(outDoc["cursor"], ShouldNotBeNil)

			_ = res1
		})
	})
}
