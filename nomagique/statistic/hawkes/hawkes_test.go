package hawkes

import (
	"math"
	"testing"
)

/*
mixedArrivals is a deterministic tape with irregular gaps: every third
arrival is a sell, the rest buys, starting on a buy.
*/
func mixedArrivals(count int) (marks []float64, times []float64) {
	at := 1_000.0

	for step := range count {
		mark := 1.0

		if (step+1)%3 == 0 {
			mark = -1.0
		}

		marks = append(marks, mark)
		times = append(times, at)
		at += float64(20+(step*37)%90) / 1_000
	}

	return marks, times
}

func TestPoissonLogLikelihoodByHand(testingT *testing.T) {
	// Origin 0 is prehistory; counted on (0, 4]: buys {0.5, 2}, sells {1.5, 4}.
	// mu_buy = mu_sell = 2/4, so LL = 2*log(1/2) - 2 + 2*log(1/2) - 2.
	stream := newArrivalStream([]float64{0, 0.5, 2}, []float64{1.5, 4})
	expected := 4*math.Log(0.5) - 4

	got, ok := poissonLogLikelihood(stream, 4)

	if !ok {
		testingT.Fatal("expected the Poisson baseline to be defined")
	}

	if math.Abs(got-expected) > 1e-12 {
		testingT.Fatalf("poisson LL = %.15f, want %.15f", got, expected)
	}

	for _, scale := range []float64{0.5, 0.9, 1.1, 2} {
		other := bivariateFit{muX: 0.5 * scale, muY: 0.5 * scale, beta: 1}
		otherLL, otherOK := other.logLikelihood(stream, 4)

		if !otherOK || otherLL >= got {
			testingT.Fatalf("rate scale %v scored %v, not below the MLE %v", scale, otherLL, got)
		}
	}
}

func TestPoissonLogLikelihoodUndefinedOneSided(testingT *testing.T) {
	stream := newArrivalStream([]float64{0, 0.5, 2, 3}, nil)

	if _, ok := poissonLogLikelihood(stream, 4); ok {
		testingT.Fatal("expected no Poisson score without counted sells")
	}
}

func TestStepUndefinedBeforeReady(testingT *testing.T) {
	model := NewHawkes()
	marks, times := mixedArrivals(3)

	for index := range marks {
		res, from, err := model.Step(marks[index], times[index])

		if err != nil {
			testingT.Fatal(err)
		}

		if model.path.modelReady {
			testingT.Fatal("expected no model from three arrivals")
		}

		if from.UnixNano() != int64(times[0]*1e9) {
			testingT.Fatalf("from = %v, want the first arrival", from)
		}

		for key := range res {
			switch key {
			case "event_count", "event_count:buy", "event_count:sell",
				"event_fraction:buy", "event_fraction:sell",
				"arrival_rate", "arrival_rate:buy", "arrival_rate:sell":
			default:
				testingT.Fatalf("model output %q emitted before a model exists", key)
			}
		}

		if _, ok := res["arrival_rate"]; ok == (index == 0) {
			testingT.Fatalf("arrival_rate defined=%v at zero span=%v", ok, index == 0)
		}
	}
}

func TestRefitDropsUnidentifiableModel(testingT *testing.T) {
	model := NewHawkes()
	marks, times := mixedArrivals(60)

	for index := range marks {
		if _, _, err := model.Step(marks[index], times[index]); err != nil {
			testingT.Fatal(err)
		}
	}

	p := model.path

	if !p.modelReady {
		testingT.Fatal("expected a fitted model from the mixed tape")
	}

	at := times[len(times)-1]

	// A retained history that is all buys cannot identify a bivariate model.
	for index := range p.samples {
		p.samples[index].mark = 1
	}

	if err := p.refit(at); err == nil {
		testingT.Fatal("expected the stale model's drop to be reported")
	}

	if p.modelReady || p.selfOnlyReady {
		testingT.Fatal("expected the stale model to be dropped")
	}

	if err := p.refit(at); err != nil {
		testingT.Fatalf("expected no repeat report without a model, got %v", err)
	}

	res, _, err := model.Step(1, at+0.05)

	if err != nil {
		testingT.Fatal(err)
	}

	for _, key := range []string{"conditional_intensity", "log_likelihood:hawkes", "compensator:buy"} {
		if _, ok := res[key]; ok {
			testingT.Fatalf("model output %q emitted after the model was dropped", key)
		}
	}
}

func TestRefitDueOnceTheFitSpanHasPassed(testingT *testing.T) {
	model := NewHawkes()
	at := 1_000.0

	// A burst at millisecond gaps, then minute gaps: a burst-era model
	// stretched over the quiet window expects thousands of arrivals.
	for step := range 90 {
		mark := 1.0

		if (step+1)%3 == 0 {
			mark = -1.0
		}

		gap := float64(5+(step*37)%20) / 1_000

		if step >= 60 {
			gap = 60 + float64((step*37)%30)
		}

		at += gap
		res, _, err := model.Step(mark, at)

		if err != nil {
			testingT.Fatal(err)
		}

		// The first quiet arrival is scored by the burst model, fitted
		// strictly before it; every later one by a model that saw the gap.
		if step <= 60 {
			continue
		}

		innovation, ok := res["count_innovation:buy"]

		if !ok {
			continue
		}

		if math.Abs(innovation) > res["event_count"] {
			testingT.Fatalf(
				"step %d: count innovation %.1f exceeds the %v retained arrivals (compensator %.1f)",
				step, innovation, res["event_count"], res["compensator:buy"],
			)
		}
	}
}

func TestDroppedReportsOncePerTransition(testingT *testing.T) {
	model := NewHawkes()
	marks, times := mixedArrivals(60)

	for index := range marks {
		if _, _, err := model.Step(marks[index], times[index]); err != nil {
			testingT.Fatal(err)
		}

		if err := model.Dropped(); err != nil {
			testingT.Fatalf("no model was dropped on the mixed tape, got %v", err)
		}
	}

	if !model.path.modelReady {
		testingT.Fatal("expected a fitted model from the mixed tape")
	}

	at := times[len(times)-1]

	for index := range model.path.samples {
		model.path.samples[index].mark = 1
	}

	reports := 0

	for step := 1; step <= 20; step++ {
		if _, _, err := model.Step(1, at+float64(step)*0.05); err != nil {
			testingT.Fatal(err)
		}

		if model.Dropped() != nil {
			reports++
		}
	}

	if reports != 1 {
		testingT.Fatalf("expected one drop report for one transition, got %d", reports)
	}
}
