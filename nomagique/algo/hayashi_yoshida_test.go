package algo

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
leg is one evaluation's arrival on both paths.
*/
type leg struct {
	from       int64
	to         int64
	leftValue  float64
	rightValue float64
}

/*
replay drives the estimator through a multi-leg path and reports the estimate,
which is how a pair correlation is actually formed: over a history, not one
observation.
*/
func replay(t *testing.T, legs []leg) HayashiYoshida_done_Results {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := HayashiYoshida_ServerToClient(NewHayashiYoshida(ctx))
	t.Cleanup(client.Release)

	for _, each := range legs {
		err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
			if err := params.SetSymbol1("MEASURED"); err != nil {
				return err
			}

			if err := params.SetSymbol2("REFERENCE"); err != nil {
				return err
			}

			params.SetBoundsStart1(float64(each.from))
			params.SetBoundsEnd1(float64(each.to))
			params.SetReturns1(each.leftValue)
			params.SetBoundsStart2(float64(each.from))
			params.SetBoundsEnd2(float64(each.to))
			params.SetReturns2(each.rightValue)
			return nil
		})
		So(err, ShouldBeNil)
	}

	future, release := client.Done(ctx, nil)
	t.Cleanup(release)

	results, err := future.Struct()
	So(err, ShouldBeNil)

	return results
}

func TestHayashiYoshidaWrite(t *testing.T) {
	Convey("Given two synchronous return paths", t, func() {
		Convey("When the paths carry identical returns", func() {
			results := replay(t, []leg{
				{from: 0, to: 1, leftValue: 0.1, rightValue: 0.1},
				{from: 1, to: 2, leftValue: -0.2, rightValue: -0.2},
				{from: 2, to: 3, leftValue: 0.3, rightValue: 0.3},
			})

			Convey("Then the paths are perfectly correlated", func() {
				So(results.Correlation(), ShouldAlmostEqual, 1.0, 1e-12)
			})

			Convey("Then the terms the estimate was formed from are reported", func() {
				energy := 0.1*0.1 + 0.2*0.2 + 0.3*0.3
				So(results.Covariance(), ShouldAlmostEqual, energy, 1e-12)
				So(results.LeftEnergy(), ShouldAlmostEqual, energy, 1e-12)
				So(results.RightEnergy(), ShouldAlmostEqual, energy, 1e-12)
				So(results.Support(), ShouldEqual, 3)
			})
		})

		Convey("When one path is the negation of the other", func() {
			results := replay(t, []leg{
				{from: 0, to: 1, leftValue: 0.1, rightValue: -0.1},
				{from: 1, to: 2, leftValue: -0.2, rightValue: 0.2},
				{from: 2, to: 3, leftValue: 0.3, rightValue: -0.3},
			})

			Convey("Then the paths are perfectly anti-correlated", func() {
				So(results.Correlation(), ShouldAlmostEqual, -1.0, 1e-12)
			})
		})

		Convey("When the returns differ in magnitude but not in sign", func() {
			results := replay(t, []leg{
				{from: 0, to: 1, leftValue: 0.1, rightValue: 0.2},
				{from: 1, to: 2, leftValue: 0.2, rightValue: 0.1},
			})

			Convey("Then the estimate is the covariance over the energy scale", func() {
				covariance := 0.1*0.2 + 0.2*0.1
				scale := math.Sqrt((0.01 + 0.04) * (0.04 + 0.01))
				So(results.Correlation(), ShouldAlmostEqual, covariance/scale, 1e-12)
			})
		})
	})

	Convey("Given return values that the estimator must actually read", t, func() {
		wide := replay(t, []leg{{from: 0, to: 1000, leftValue: 0.1, rightValue: 0.1}})
		narrow := replay(t, []leg{{from: 0, to: 2, leftValue: 0.1, rightValue: 0.1}})

		Convey("When two paths differ only in how long their intervals are", func() {
			Convey("Then the estimate follows the returns, not the elapsed time", func() {
				So(wide.Covariance(), ShouldAlmostEqual, narrow.Covariance(), 1e-12)
				So(wide.Covariance(), ShouldAlmostEqual, 0.01, 1e-12)
			})
		})
	})

	Convey("Given paths whose return intervals never meet", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := HayashiYoshida_ServerToClient(NewHayashiYoshida(ctx))
		defer client.Release()

		err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
			if err := params.SetSymbol1("MEASURED"); err != nil {
				return err
			}

			if err := params.SetSymbol2("REFERENCE"); err != nil {
				return err
			}

			params.SetBoundsStart1(0)
			params.SetBoundsEnd1(1)
			params.SetReturns1(0.5)
			params.SetBoundsStart2(5)
			params.SetBoundsEnd2(6)
			params.SetReturns2(0.5)
			return nil
		})
		So(err, ShouldBeNil)

		future, release := client.Done(ctx, nil)
		defer release()

		results, err := future.Struct()
		So(err, ShouldBeNil)

		Convey("Then no overlap contributes and the covariance stays zero", func() {
			So(results.Support(), ShouldEqual, 0)
			So(results.Covariance(), ShouldEqual, 0.0)
		})
	})

	Convey("Given a return interval that ends before it starts", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := HayashiYoshida_ServerToClient(NewHayashiYoshida(ctx))
		defer client.Release()

		err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
			if err := params.SetSymbol1("MEASURED"); err != nil {
				return err
			}

			if err := params.SetSymbol2("REFERENCE"); err != nil {
				return err
			}

			params.SetBoundsStart1(10)
			params.SetBoundsEnd1(1)
			params.SetReturns1(0.5)
			params.SetBoundsStart2(0)
			params.SetBoundsEnd2(1)
			params.SetReturns2(0.5)
			return nil
		})
		So(err, ShouldBeNil)

		Convey("Then time regression surfaces at the streaming fence", func() {
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func TestHayashiYoshidaDone(t *testing.T) {
	Convey("Given one estimator carrying several symbol pairs at once", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := HayashiYoshida_ServerToClient(NewHayashiYoshida(ctx))
		defer client.Release()

		/*
			write feeds one leg of one pair, so the pairs interleave the way
			independent symbols actually arrive on a shared feed.
		*/
		write := func(measured, reference string, from, to int64, left, right float64) {
			err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
				if err := params.SetSymbol1(measured); err != nil {
					return err
				}

				if err := params.SetSymbol2(reference); err != nil {
					return err
				}

				params.SetBoundsStart1(float64(from))
				params.SetBoundsEnd1(float64(to))
				params.SetReturns1(left)
				params.SetBoundsStart2(float64(from))
				params.SetBoundsEnd2(float64(to))
				params.SetReturns2(right)
				return nil
			})
			So(err, ShouldBeNil)
		}

		/*
			read takes the estimate before the results are released, because a
			Cap'n Proto result struct does not outlive its call.
		*/
		read := func() (correlation, support float64) {
			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results.Correlation(), results.Support()
		}

		Convey("When a correlated pair interleaves with an anti-correlated pair", func() {
			write("BTC", "ETH", 0, 1, 0.1, 0.1)
			write("SOL", "ETH", 0, 1, 0.1, -0.1)
			write("BTC", "ETH", 1, 2, 0.2, 0.2)
			write("SOL", "ETH", 1, 2, 0.2, -0.2)

			Convey("Then each pair reports its own accumulation", func() {
				write("BTC", "ETH", 2, 3, 0.3, 0.3)
				correlated, _ := read()
				So(correlated, ShouldAlmostEqual, 1.0, 1e-12)

				write("SOL", "ETH", 2, 3, 0.3, -0.3)
				opposed, _ := read()
				So(opposed, ShouldAlmostEqual, -1.0, 1e-12)
			})
		})

		Convey("When a pair is measured in the opposite orientation", func() {
			write("BTC", "ETH", 0, 1, 0.1, 0.2)

			Convey("Then provenance is preserved as a distinct accumulation", func() {
				_, forward := read()
				So(forward, ShouldEqual, 1)

				write("ETH", "BTC", 0, 1, 0.2, 0.1)
				_, reversed := read()
				So(reversed, ShouldEqual, 1)
			})
		})
	})
}
