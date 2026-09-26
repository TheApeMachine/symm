package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestOtsuWrite(t *testing.T) {
	ctx := context.Background()

	Convey("Given labelled activations", t, func() {
		client := statistic.Otsu_ServerToClient(statistic.NewOtsu())
		defer client.Release()

		split := func(values []float64, labels []string) []string {
			So(client.Write(ctx, func(params statistic.Otsu_write_Params) error {
				numbers, err := params.NewValues(int32(len(values)))

				if err != nil {
					return err
				}

				names, err := params.NewLabels(int32(len(labels)))

				if err != nil {
					return err
				}

				for element := range values {
					numbers.Set(element, values[element])

					if err := names.Set(element, labels[element]); err != nil {
						return err
					}
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			hot, err := results.Hot()
			So(err, ShouldBeNil)

			labelled := []string{}

			for position := range hot.Len() {
				label, err := hot.At(position)
				So(err, ShouldBeNil)
				labelled = append(labelled, label)
			}

			return labelled
		}

		Convey("The strong class is split from the rest where the variance between them peaks", func() {
			hot := split([]float64{0.1, 9, 0.2, 8.5, 0.15}, []string{"a", "b", "c", "d", "e"})
			So(hot, ShouldResemble, []string{"b", "d"})

			Convey("And the next evaluation starts clean", func() {
				hot := split(nil, nil)
				So(hot, ShouldBeEmpty)
			})
		})

		Convey("Nothing lit is no strong class", func() {
			hot := split([]float64{0, 0}, []string{"a", "b"})
			So(hot, ShouldBeEmpty)
		})

		Convey("A single candidate is the strong class on its own", func() {
			hot := split([]float64{0, 3}, []string{"a", "b"})
			So(hot, ShouldResemble, []string{"b"})
		})
	})
}
