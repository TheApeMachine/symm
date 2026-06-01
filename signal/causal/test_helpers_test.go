package causal

import "time"

func ladderTrainingSamples(count int) []causalSample {
	samples := make([]causalSample, 0, count)

	for index := range count {
		flow := 1 + float64(index)*0.05
		liquidity := 70 + float64(index%7)*8
		macro := 0.2 + float64(index%3)*0.05
		velocity := flow*0.006 + macro*0.001 + float64(index%5)*0.0003

		samples = append(samples, newCausalSample(macro, liquidity, flow, velocity))
	}

	return samples
}

func seedLadderHistory(state *CausalSymbol, count int) {
	state.mu.Lock()
	state.samples = ladderTrainingSamples(count)
	state.mu.Unlock()
}

func collinearTrainingSamples(count int) []causalSample {
	samples := make([]causalSample, 0, count)

	for index := range count {
		flow := 1 + float64(index)*0.03
		liquidity := flow * 40
		macro := 0.1
		velocity := flow * 0.01

		samples = append(samples, newCausalSample(macro, liquidity, flow, velocity))
	}

	return samples
}

func upliftTrainingSamples(count int) []causalSample {
	samples := make([]causalSample, 0, count)

	for index := range count {
		flow := 1 + float64(index)*0.1
		liquidity := 55 + float64(index%6)*10
		macro := 0.15 + float64(index%4)*0.03
		velocity := 0.015*flow*flow + macro*0.002

		samples = append(samples, newCausalSample(macro, liquidity, flow, velocity))
	}

	return samples
}

func tableFromSamples(samples []causalSample) dagNodeTable {
	table, err := causalTable(samples)

	if err != nil {
		panic(err)
	}

	return table
}

func correlatedHYSeries(base int64, prices []float64) *hyReturns {
	series := newHYReturns(len(prices) + 4)

	for index, price := range prices {
		series.Observe(base+int64(index)*int64(time.Millisecond), price)
	}

	return series
}
