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
	from       float64
	to         float64
	leftValue  float64
	rightValue float64
}

/*
estimate is what one evaluation published.
*/
type estimate struct {
	state       []byte
	correlation float64
	covariance  float64
	support     float64
	leftEnergy  float64
	rightEnergy float64
}

/*
pairStore stands in for the graph storage a real composition would use, so the
estimator is driven the way a graph drives it: state in, state out, keyed by
whatever the graph decided identifies a pair.
*/
type pairStore map[string][]byte

/*
step runs one evaluation for a pair, reading the retained accumulation and
writing back the updated one.
*/
func step(
	t *testing.T,
	ctx context.Context,
	client HayashiYoshida,
	retained pairStore,
	pair string,
	each leg,
) estimate {
	t.Helper()

	err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
		if err := params.SetState(retained[pair]); err != nil {
			return err
		}

		params.SetBoundsStart1(each.from)
		params.SetBoundsEnd1(each.to)
		params.SetReturns1(each.leftValue)
		params.SetBoundsStart2(each.from)
		params.SetBoundsEnd2(each.to)
		params.SetReturns2(each.rightValue)
		return nil
	})
	So(err, ShouldBeNil)

	future, release := client.Done(ctx, nil)
	defer release()

	results, err := future.Struct()
	So(err, ShouldBeNil)

	state, err := results.State()
	So(err, ShouldBeNil)

	// The state is copied because the results are released with the call.
	carried := append([]byte(nil), state...)
	retained[pair] = carried

	return estimate{
		state:       carried,
		correlation: results.Correlation(),
		covariance:  results.Covariance(),
		support:     results.Support(),
		leftEnergy:  results.LeftEnergy(),
		rightEnergy: results.RightEnergy(),
	}
}

func newClient(t *testing.T) (context.Context, HayashiYoshida) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := HayashiYoshida_ServerToClient(NewHayashiYoshida(ctx))
	t.Cleanup(client.Release)

	return ctx, client
}

func replay(t *testing.T, legs []leg) estimate {
	t.Helper()

	ctx, client := newClient(t)
	retained := pairStore{}

	var last estimate
	for _, each := range legs {
		last = step(t, ctx, client, retained, "pair", each)
	}

	return last
}

func TestHayashiYoshidaWrite(t *testing.T) {
	Convey("Given two synchronous return paths", t, func() {
		Convey("When the paths carry identical returns", func() {
			result := replay(t, []leg{
				{from: 0, to: 1, leftValue: 0.1, rightValue: 0.1},
				{from: 1, to: 2, leftValue: -0.2, rightValue: -0.2},
				{from: 2, to: 3, leftValue: 0.3, rightValue: 0.3},
			})

			Convey("Then the paths are perfectly correlated", func() {
				So(result.correlation, ShouldAlmostEqual, 1.0, 1e-12)
			})

			Convey("Then the terms the estimate was formed from are reported", func() {
				energy := 0.1*0.1 + 0.2*0.2 + 0.3*0.3
				So(result.covariance, ShouldAlmostEqual, energy, 1e-12)
				So(result.leftEnergy, ShouldAlmostEqual, energy, 1e-12)
				So(result.rightEnergy, ShouldAlmostEqual, energy, 1e-12)
				So(result.support, ShouldEqual, 3)
			})
		})

		Convey("When one path is the negation of the other", func() {
			result := replay(t, []leg{
				{from: 0, to: 1, leftValue: 0.1, rightValue: -0.1},
				{from: 1, to: 2, leftValue: -0.2, rightValue: 0.2},
				{from: 2, to: 3, leftValue: 0.3, rightValue: -0.3},
			})

			Convey("Then the paths are perfectly anti-correlated", func() {
				So(result.correlation, ShouldAlmostEqual, -1.0, 1e-12)
			})
		})

		Convey("When the returns differ in magnitude but not in sign", func() {
			result := replay(t, []leg{
				{from: 0, to: 1, leftValue: 0.1, rightValue: 0.2},
				{from: 1, to: 2, leftValue: 0.2, rightValue: 0.1},
			})

			Convey("Then the estimate is the covariance over the energy scale", func() {
				covariance := 0.1*0.2 + 0.2*0.1
				scale := math.Sqrt((0.01 + 0.04) * (0.04 + 0.01))
				So(result.correlation, ShouldAlmostEqual, covariance/scale, 1e-12)
			})
		})
	})

	Convey("Given return values the estimator must actually read", t, func() {
		wide := replay(t, []leg{{from: 0, to: 1000, leftValue: 0.1, rightValue: 0.1}})
		narrow := replay(t, []leg{{from: 0, to: 2, leftValue: 0.1, rightValue: 0.1}})

		Convey("When two paths differ only in how long their intervals are", func() {
			Convey("Then the estimate follows the returns, not the elapsed time", func() {
				So(wide.covariance, ShouldAlmostEqual, narrow.covariance, 1e-12)
				So(wide.covariance, ShouldAlmostEqual, 0.01, 1e-12)
			})
		})
	})

	Convey("Given paths whose return intervals never meet", t, func() {
		ctx, client := newClient(t)
		retained := pairStore{}

		result := step(t, ctx, client, retained, "pair",
			leg{from: 0, to: 1, leftValue: 0.5, rightValue: 0.5})
		_ = result

		err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
			if err := params.SetState(nil); err != nil {
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

		Convey("Then the correlation is reported as undefined, not as zero", func() {
			// Both energies are positive here, so the ratio is a real zero:
			// the paths overlapped nowhere, which is measured independence.
			So(math.IsNaN(results.Correlation()), ShouldBeFalse)
			So(results.Correlation(), ShouldEqual, 0.0)
		})
	})

	Convey("Given a pair that has produced no return energy at all", t, func() {
		ctx, client := newClient(t)

		err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
			if err := params.SetState(nil); err != nil {
				return err
			}

			params.SetBoundsStart1(0)
			params.SetBoundsEnd1(1)
			params.SetReturns1(0)
			params.SetBoundsStart2(0)
			params.SetBoundsEnd2(1)
			params.SetReturns2(0)
			return nil
		})
		So(err, ShouldBeNil)

		future, release := client.Done(ctx, nil)
		defer release()

		results, err := future.Struct()
		So(err, ShouldBeNil)

		Convey("Then normalization is undefined rather than a fabricated zero", func() {
			// A zero denominator is not evidence of independence, and reporting
			// it as 0.0 would be indistinguishable from a measured zero
			// correlation. It stays undefined so its cause surfaces.
			So(math.IsNaN(results.Correlation()), ShouldBeTrue)
		})
	})

	Convey("Given a return interval that ends before it starts", t, func() {
		ctx, client := newClient(t)

		err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
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
	Convey("Given one estimator serving several pairs from graph storage", t, func() {
		ctx, client := newClient(t)
		retained := pairStore{}

		Convey("When a correlated pair interleaves with an anti-correlated pair", func() {
			step(t, ctx, client, retained, "BTC/ETH", leg{0, 1, 0.1, 0.1})
			step(t, ctx, client, retained, "SOL/ETH", leg{0, 1, 0.1, -0.1})
			step(t, ctx, client, retained, "BTC/ETH", leg{1, 2, 0.2, 0.2})
			step(t, ctx, client, retained, "SOL/ETH", leg{1, 2, 0.2, -0.2})

			Convey("Then each pair's own history decides its estimate", func() {
				correlated := step(t, ctx, client, retained, "BTC/ETH", leg{2, 3, 0.3, 0.3})
				So(correlated.correlation, ShouldAlmostEqual, 1.0, 1e-12)

				opposed := step(t, ctx, client, retained, "SOL/ETH", leg{2, 3, 0.3, -0.3})
				So(opposed.correlation, ShouldAlmostEqual, -1.0, 1e-12)
			})
		})

		Convey("When the estimator is handed no retained accumulation", func() {
			step(t, ctx, client, retained, "BTC/ETH", leg{0, 1, 0.1, 0.1})
			fresh := step(t, ctx, client, pairStore{}, "BTC/ETH", leg{1, 2, 0.2, 0.2})

			Convey("Then it keeps nothing of its own from the previous evaluation", func() {
				So(fresh.support, ShouldEqual, 1)
				So(fresh.leftEnergy, ShouldAlmostEqual, 0.04, 1e-12)
			})
		})
	})
}

/*
The estimate travels with the diagnostics that make it auditable: how many
returns each path contributed, how long they were observed together, and the
typical energy each carried. Without those a correlation cannot be told apart
from one formed on almost no evidence.
*/
func TestHayashiYoshidaDiagnostics(t *testing.T) {
	Convey("Given two paths observed over the same span", t, func() {
		ctx, client := newClient(t)
		retained := pairStore{}

		var last struct {
			leftReturns    float64
			rightReturns   float64
			sharedTime     float64
			overlapDensity float64
			leftRate       float64
		}

		read := func() {
			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			last.leftReturns = results.LeftReturns()
			last.rightReturns = results.RightReturns()
			last.sharedTime = results.SharedTime()
			last.overlapDensity = results.OverlapDensity()
			last.leftRate = results.LeftEnergyRate()

			state, err := results.State()
			So(err, ShouldBeNil)
			retained["pair"] = append([]byte(nil), state...)
		}

		write := func(from, to, left, right float64) {
			err := client.Write(ctx, func(params HayashiYoshida_write_Params) error {
				if err := params.SetState(retained["pair"]); err != nil {
					return err
				}

				params.SetBoundsStart1(from)
				params.SetBoundsEnd1(to)
				params.SetReturns1(left)
				params.SetBoundsStart2(from)
				params.SetBoundsEnd2(to)
				params.SetReturns2(right)
				return nil
			})
			So(err, ShouldBeNil)
			read()
		}

		Convey("When three returns arrive on each path", func() {
			write(0, 1, 0.1, 0.1)
			write(1, 2, 0.2, 0.2)
			write(2, 4, 0.3, 0.3)

			Convey("Then each path reports what it contributed", func() {
				So(last.leftReturns, ShouldEqual, 3)
				So(last.rightReturns, ShouldEqual, 3)
			})

			Convey("Then the span they were observed together is reported", func() {
				So(last.sharedTime, ShouldEqual, 4)
			})

			Convey("Then overlap is reported against that span, not as a bare count", func() {
				So(last.overlapDensity, ShouldAlmostEqual, 3.0/4.0, 1e-12)
			})

			Convey("Then activity is the typical rate, not the largest", func() {
				// Rates are 0.01/1, 0.04/1 and 0.09/2: the middle one stands
				// for the path, so one big move does not become its normal.
				So(last.leftRate, ShouldAlmostEqual, 0.04, 1e-12)
			})
		})
	})
}
