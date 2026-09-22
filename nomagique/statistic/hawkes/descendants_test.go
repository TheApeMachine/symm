package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
descendantsOf drives the Descendants node over one branching matrix.
*/
func descendantsOf(t *testing.T, branching []float64, dimension int) ([]float64, bool) {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Descendants_ServerToClient(hawkes.NewDescendants())

	err := client.Write(ctx, func(params hawkes.Descendants_write_Params) error {
		if err := writeFloats(params.NewBranching, branching); err != nil {
			return err
		}

		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("descendants write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("descendants stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("descendants done: %v", err)
	}

	list, err := results.Descendants()

	if err != nil {
		t.Fatalf("descendants list: %v", err)
	}

	return readList(list.Len(), list.At), results.Defined()
}

/*
generations sums the first depth generations of offspring a single arrival on
each component produces, by repeatedly multiplying the branching matrix. It
is the definition the closed form is supposed to be equal to.
*/
func generations(branching []float64, dimension, depth int) []float64 {
	power := append([]float64(nil), branching...)
	total := make([]float64, dimension)

	for generation := 0; generation < depth; generation++ {
		for column := 0; column < dimension; column++ {
			for row := 0; row < dimension; row++ {
				total[column] += power[row*dimension+column]
			}
		}

		next := make([]float64, dimension*dimension)

		for row := 0; row < dimension; row++ {
			for column := 0; column < dimension; column++ {
				for inner := 0; inner < dimension; inner++ {
					next[row*dimension+column] += power[row*dimension+inner] * branching[inner*dimension+column]
				}
			}
		}

		power = next
	}

	return total
}

func TestDescendantsServer_Write(t *testing.T) {
	Convey("Given a subcritical branching matrix", t, func() {
		branching := []float64{0.30, 0.15, 0.20, 0.25}

		Convey("When the expected progeny is computed", func() {
			descendants, defined := descendantsOf(t, branching, 2)

			Convey("Then it is defined", func() {
				So(defined, ShouldBeTrue)
				So(len(descendants), ShouldEqual, 2)
			})

			Convey("Then it equals the summed generations it stands for", func() {
				// Deep enough that the remaining generations are far below
				// the tolerance, since each one shrinks by the spectral
				// radius.
				summed := generations(branching, 2, 400)

				for index := range descendants {
					So(math.Abs(descendants[index]-summed[index]), ShouldBeLessThan, 1e-9)
				}
			})

			Convey("Then it counts more than the first generation alone", func() {
				first := generations(branching, 2, 1)

				for index := range descendants {
					So(descendants[index], ShouldBeGreaterThan, first[index])
				}
			})
		})

		Convey("When the matrix is critical or beyond", func() {
			_, defined := descendantsOf(t, []float64{1.0, 0.2, 0.2, 1.0}, 2)

			Convey("Then no finite progeny exists and none is reported", func() {
				So(defined, ShouldBeFalse)
			})
		})

		Convey("When a component does not excite anything", func() {
			descendants, defined := descendantsOf(t, []float64{0.5, 0, 0, 0}, 2)

			Convey("Then it has no descendants while the other still does", func() {
				So(defined, ShouldBeTrue)
				So(descendants[1], ShouldEqual, 0)
				So(descendants[0], ShouldBeGreaterThan, 0)
			})
		})
	})
}
