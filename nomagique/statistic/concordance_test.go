package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestConcordance(t *testing.T) {
	ctx := context.Background()

	Convey("Given the concordance of one pair of elements", t, func() {
		client := statistic.Concordance_ServerToClient(statistic.NewConcordance())
		defer client.Release()

		record := make([]float64, 5)
		known := false

		// read feeds one paired movement and carries the pair's state
		// forward the way its store does, returning strength and orientation
		// when the pair has support.
		// silent names an end that did not report: its movement is left unread.
		read := func(left, right float64, silent ...int) (float64, float64, bool) {
			So(client.Write(ctx, func(params statistic.Concordance_write_Params) error {
				defined, err := params.NewDefined(2)

				if err != nil {
					return err
				}

				defined.Set(0, true)
				defined.Set(1, true)

				for _, end := range silent {
					defined.Set(end, false)
				}

				value, err := params.NewValue(2)

				if err != nil {
					return err
				}

				value.Set(0, left)
				value.Set(1, right)

				from, err := params.NewFromNodes(1)

				if err != nil {
					return err
				}

				to, err := params.NewToNodes(1)

				if err != nil {
					return err
				}

				pairs, err := params.NewPairs(1)

				if err != nil {
					return err
				}

				from.Set(0, 0)
				to.Set(0, 1)
				pairs.Set(0, 1)

				prior, err := params.NewPrior(5)

				if err != nil {
					return err
				}

				for offset, number := range record {
					prior.Set(offset, number)
				}

				flags, err := params.NewKnown(1)

				if err != nil {
					return err
				}

				flags.Set(0, known)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			state, err := results.State()
			So(err, ShouldBeNil)

			if state.Len() == 5 {
				known = true

				for offset := range 5 {
					record[offset] = state.At(offset)
				}
			}

			strength, err := results.Strength()
			So(err, ShouldBeNil)
			orientation, err := results.Orientation()
			So(err, ShouldBeNil)

			if strength.Len() == 0 {
				return 0, 0, false
			}

			return strength.At(0), orientation.At(0), true
		}

		Convey("Neither element moving says nothing about the pair", func() {
			_, _, emitted := read(0, 0)
			So(emitted, ShouldBeFalse)
			So(known, ShouldBeFalse)
		})

		Convey("A pair with no history learns nothing from one end's silence", func() {
			_, _, emitted := read(1, 0, 1)
			So(emitted, ShouldBeFalse)
			So(known, ShouldBeFalse)
		})

		Convey("Consistently inverse movement attracts, oriented inverse", func() {
			var strength, orientation float64

			for _, movement := range []float64{1, -2, 1.5, -1, 2} {
				strength, orientation, _ = read(movement, -movement)
			}

			So(strength, ShouldBeGreaterThan, 0)
			So(orientation, ShouldEqual, -1)

			Convey("And is as strong as the same movement taken directly", func() {
				inverse := strength
				clear(record)
				known = false

				for _, movement := range []float64{1, -2, 1.5, -1, 2} {
					strength, orientation, _ = read(movement, movement)
				}

				So(strength, ShouldAlmostEqual, inverse)
				So(orientation, ShouldEqual, 1)
			})

			Convey("And an end that does not report at all, while the other moves, is a non-response that repels", func() {
				strength, _, emitted := read(1, 0, 1)
				So(emitted, ShouldBeTrue)
				So(strength, ShouldBeLessThan, 0)
			})

			Convey("And one observed non-response breaks it and repels", func() {
				strength, _, _ := read(1, 0)
				So(strength, ShouldBeLessThan, 0)
			})

			Convey("And a sign that now agrees where it used to oppose repels", func() {
				strength, _, _ := read(1, 1)
				So(strength, ShouldBeLessThan, 0)
			})
		})

		Convey("Alternating agreement and disagreement is no relationship and repels", func() {
			var strength float64

			for step, movement := range []float64{1, 1, 1, 1, 1, 1} {
				other := movement

				if step%2 == 1 {
					other = -movement
				}

				strength, _, _ = read(movement, other)
			}

			So(strength, ShouldBeLessThan, 0)
		})

		Convey("Matching magnitudes attract more than mismatched ones", func() {
			var matched float64

			for _, movement := range []float64{1, 2, 1, 2} {
				matched, _, _ = read(movement, movement)
			}

			clear(record)
			known = false
			var mismatched float64

			for _, movement := range []float64{1, 2, 1, 2} {
				mismatched, _, _ = read(movement, movement*10)
			}

			So(matched, ShouldBeGreaterThan, mismatched)
			So(mismatched, ShouldBeGreaterThan, 0)
		})
	})
}
