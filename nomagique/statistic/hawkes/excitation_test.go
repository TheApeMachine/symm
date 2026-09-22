package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
excitationOf drives the Excitation node over one window at one horizon.
*/
func excitationOf(t *testing.T, path realisation, horizon, decay float64) []float64 {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Excitation_ServerToClient(hawkes.NewExcitation())

	err := client.Write(ctx, func(params hawkes.Excitation_write_Params) error {
		if err := writeFloats(params.NewTimes, path.times); err != nil {
			return err
		}

		if err := writeFloats(params.NewComponents, path.components); err != nil {
			return err
		}

		params.SetHorizon(horizon)
		params.SetDecay(decay)
		params.SetDimension(int32(path.dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("excitation write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("excitation stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("excitation done: %v", err)
	}

	list, err := results.Support()

	if err != nil {
		t.Fatalf("excitation list: %v", err)
	}

	return readList(list.Len(), list.At)
}

func TestExcitationServer_Write(t *testing.T) {
	Convey("Given arrivals on two components", t, func() {
		path := realisation{
			times:      []float64{0, 1, 2, 3},
			components: []float64{0, 1, 0, 1},
			dimension:  2,
		}

		Convey("When the support is read one decay constant after the last arrival", func() {
			support := excitationOf(t, path, 4, 1.0)

			Convey("Then each component carries its own arrivals only", func() {
				So(math.Abs(support[0]-(math.Exp(-4)+math.Exp(-2))), ShouldBeLessThan, 1e-12)
				So(math.Abs(support[1]-(math.Exp(-3)+math.Exp(-1))), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When an arrival sits exactly on the horizon", func() {
			support := excitationOf(t, path, 3, 1.0)

			Convey("Then it does not excite its own instant", func() {
				// Component 1's arrivals are at 1 and 3. Only the one at 1
				// may contribute; letting the arrival at 3 excite the
				// intensity that governs it would count it twice.
				So(math.Abs(support[1]-math.Exp(-2)), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When the horizon advances by one half-life", func() {
			decay := math.Ln2
			near := excitationOf(t, path, 4, decay)
			far := excitationOf(t, path, 5, decay)

			Convey("Then the standing support halves", func() {
				So(math.Abs(far[0]-near[0]/2), ShouldBeLessThan, 1e-12)
				So(math.Abs(far[1]-near[1]/2), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When the horizon precedes every arrival", func() {
			support := excitationOf(t, path, -1, 1.0)

			Convey("Then nothing has happened yet to excite anything", func() {
				So(support, ShouldResemble, []float64{0, 0})
			})
		})

		Convey("When the decay rate is not positive", func() {
			support := excitationOf(t, path, 4, 0)

			Convey("Then the kernel has no scale and no support is reported", func() {
				So(support, ShouldBeEmpty)
			})
		})

		Convey("When a component label falls outside the process", func() {
			strayed := realisation{
				times:      []float64{0, 1},
				components: []float64{0, 5},
				dimension:  2,
			}
			support := excitationOf(t, strayed, 2, 1.0)

			Convey("Then the stray arrival is not attributed to another component", func() {
				So(support[1], ShouldEqual, 0)
				So(math.Abs(support[0]-math.Exp(-2)), ShouldBeLessThan, 1e-12)
			})
		})
	})
}
