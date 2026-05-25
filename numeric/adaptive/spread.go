package adaptive

type Spread struct {
	smoother *EMA
}

func NewSpread(rate float64) *Spread {
	return &Spread{smoother: NewEMA(rate)}
}

func (spread *Spread) Next(out float64, values ...float64) (float64, error) {
	var err error

	ref := out

	for _, observation := range values {
		deviation := observation - ref
		out, err = spread.smoother.Next(0, deviation*deviation)

		if err != nil {
			return 0, err
		}
	}

	return out, nil
}

func (spread *Spread) Reset() error {
	return spread.smoother.Reset()
}
