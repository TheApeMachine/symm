package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
authorityReading is one evaluation of Authority, with its retained state
carried the way a store written back as feedback carries it.
*/
type authorityReading struct {
	standard  []float64
	defined   []bool
	authority []float64
	maturity  []float64
	snr       []float64
	energy    []float64
}

func TestAuthority(t *testing.T) {
	ctx := context.Background()

	Convey("Given the authority of two elements read over time", t, func() {
		client := statistic.Authority_ServerToClient(statistic.NewAuthority())
		defer client.Release()

		records := make([]float64, 6)
		known := make([]bool, 2)

		read := func(value []float64, defined []bool) authorityReading {
			So(client.Write(ctx, func(params statistic.Authority_write_Params) error {
				values, err := params.NewValue(int32(len(value)))

				if err != nil {
					return err
				}

				flags, err := params.NewDefined(int32(len(defined)))

				if err != nil {
					return err
				}

				for element := range value {
					values.Set(element, value[element])
					flags.Set(element, defined[element])
				}

				prior, err := params.NewPrior(int32(len(records)))

				if err != nil {
					return err
				}

				for offset, number := range records {
					prior.Set(offset, number)
				}

				knowns, err := params.NewKnown(int32(len(known)))

				if err != nil {
					return err
				}

				for element, flag := range known {
					knowns.Set(element, flag)
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			index, err := results.Index()
			So(err, ShouldBeNil)
			state, err := results.State()
			So(err, ShouldBeNil)

			for position := range index.Len() {
				element := int(index.At(position))
				known[element] = true

				for offset := range 3 {
					records[element*3+offset] = state.At(position*3 + offset)
				}
			}

			reading := authorityReading{}
			standard, err := results.Standard()
			So(err, ShouldBeNil)
			flags, err := results.Defined()
			So(err, ShouldBeNil)
			authority, err := results.Authority()
			So(err, ShouldBeNil)
			energy, err := results.Energy()
			So(err, ShouldBeNil)
			maturity, err := results.Maturity()
			So(err, ShouldBeNil)
			snr, err := results.Snr()
			So(err, ShouldBeNil)

			for element := range standard.Len() {
				reading.standard = append(reading.standard, standard.At(element))
				reading.defined = append(reading.defined, flags.At(element))
				reading.authority = append(reading.authority, authority.At(element))
				reading.energy = append(reading.energy, energy.At(element))
				reading.maturity = append(reading.maturity, maturity.At(element))
				reading.snr = append(reading.snr, snr.At(element))
			}

			return reading
		}

		Convey("A first reading has no scale to be measured against, and no maturity", func() {
			reading := read([]float64{2, 1}, []bool{true, true})
			So(reading.defined, ShouldResemble, []bool{false, false})
			So(reading.maturity, ShouldResemble, []float64{0, 0})
			So(reading.authority, ShouldResemble, []float64{0, 0})

			Convey("A later reading is scaled by the element's own earlier movement", func() {
				reading := read([]float64{4, 0}, []bool{true, true})
				So(reading.defined, ShouldResemble, []bool{true, true})
				So(reading.standard[0], ShouldEqual, 2)

				Convey("And a reading of exactly zero stays zero: it was read and did not move", func() {
					So(reading.standard[1], ShouldEqual, 0)
					So(reading.authority[1], ShouldEqual, 0)
				})

				Convey("And authority is maturity times mean SNR fraction, as data.Quality defines them", func() {
					// Two readings: maturity 1-1/2. The second read 4 against a mean
					// square of 4, an SNR of 4 and a fraction of 4/5, over 2 readings.
					So(reading.maturity[0], ShouldAlmostEqual, 0.5)
					So(reading.snr[0], ShouldAlmostEqual, (4.0/5.0)/2.0)
					So(reading.authority[0], ShouldAlmostEqual, 0.5*(4.0/5.0)/2.0)
					So(reading.energy[0], ShouldAlmostEqual, 4*reading.authority[0])
				})

				Convey("An element left unread keeps its earned authority but lights nothing", func() {
					reading := read([]float64{3, 0}, []bool{true, false})
					So(reading.defined[1], ShouldBeFalse)
					So(reading.energy[1], ShouldEqual, 0)

					Convey("And authority falls behind for the element with less evidence", func() {
						So(reading.authority[1], ShouldBeLessThan, reading.authority[0])
					})
				})
			})
		})
	})
}
