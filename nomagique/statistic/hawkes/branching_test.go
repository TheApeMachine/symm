package hawkes_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
branchingOf drives the Branching node over one excitation matrix.
*/
func branchingOf(t *testing.T, excitation []float64, decay float64, dimension int) []float64 {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Branching_ServerToClient(hawkes.NewBranching())

	err := client.Write(ctx, func(params hawkes.Branching_write_Params) error {
		if err := writeFloats(params.NewExcitation, excitation); err != nil {
			return err
		}

		params.SetDecay(decay)
		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("branching write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("branching stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("branching done: %v", err)
	}

	list, err := results.Branching()

	if err != nil {
		t.Fatalf("branching list: %v", err)
	}

	return readList(list.Len(), list.At)
}

func TestBranchingServer_Write(t *testing.T) {
	Convey("Given an excitation matrix and a decay rate", t, func() {
		excitation := []float64{0.8, 0.2, 0.4, 0.6}

		Convey("When the branching matrix is formed", func() {
			branching := branchingOf(t, excitation, 2.0, 2)

			Convey("Then each entry is its excitation integrated over all future time", func() {
				So(branching, ShouldResemble, []float64{0.4, 0.1, 0.2, 0.3})
			})
		})

		Convey("When the decay rate is not positive", func() {
			branching := branchingOf(t, excitation, 0, 2)

			Convey("Then the kernel never decays, so no finite branching exists", func() {
				So(branching, ShouldBeEmpty)
			})
		})

		Convey("When the matrix is smaller than the stated dimension", func() {
			branching := branchingOf(t, []float64{0.5}, 2.0, 2)

			Convey("Then nothing is reported rather than a padded matrix", func() {
				So(branching, ShouldBeEmpty)
			})
		})
	})
}
