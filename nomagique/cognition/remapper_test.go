package cognition_test

import (
	"context"
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

		Convey("Initial evaluation establishes baseline and reports settled status", func() {
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

			So(results.Settled(), ShouldBeTrue)
			So(results.Revision(), ShouldEqual, 1)

			vocab, err := results.Vocabulary()
			So(err, ShouldBeNil)
			So(vocab, ShouldNotBeEmpty)

			tokens, err := results.Tokens()
			So(err, ShouldBeNil)
			So(tokens.Len(), ShouldBeGreaterThan, 0)
		})

		Convey("Unsettled state emits no hot region tokens", func() {
			// Supply strong sympathy pull between distant metrics that triggers movement
			evidenceJSON := []byte(`{
				"metric_a:metric_d": {"sympathy": -100.0, "magnitude": 10.0}
			}`)

			err := client.Write(ctx, func(params cognition.Remapper_write_Params) error {
				params.SetReset(true)
				ids, _ := params.NewIds(4)
				ids.Set(0, "metric_a")
				ids.Set(1, "metric_b")
				ids.Set(2, "metric_c")
				ids.Set(3, "metric_d")

				acts, _ := params.NewActivations(4)
				acts.Set(0, 1.0)
				acts.Set(1, 1.0)
				acts.Set(2, 1.0)
				acts.Set(3, 1.0)

				auths, _ := params.NewAuthorities(4)
				auths.Set(0, 0.1)
				auths.Set(1, 0.1)
				auths.Set(2, 0.1)
				auths.Set(3, 0.1)

				params.SetEvidence(evidenceJSON)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			// If swaps occurred, it was moving so not settled in this pass
			if !results.Settled() {
				tokens, err := results.Tokens()
				So(err, ShouldBeNil)
				So(tokens.Len(), ShouldEqual, 0)
			}
		})
	})
}
