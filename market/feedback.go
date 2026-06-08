package market

type Feedback struct {
	Symbol  string
	MSE     float64
	Scale   float64
	Bias    float64
	Samples int
}

func NewFeedback(
	symbol string,
	mse float64,
	scale float64,
	bias float64,
	samples int,
) *Feedback {
	if mse < 0 {
		mse = 0
	}

	return &Feedback{
		Symbol:  symbol,
		MSE:     mse,
		Scale:   scale,
		Bias:    bias,
		Samples: samples,
	}
}
