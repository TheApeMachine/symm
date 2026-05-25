package adaptive

import "fmt"

/*
AlphaEMA is a plain exponential moving average with an explicit smoothing
coefficient supplied per update.
*/
type AlphaEMA struct {
	value   float64
	updates int
}

/*
Update folds one observation in using alpha in (0, 1]. The first observation
seeds the tracker so downstream Value calls are stable immediately.
*/
func (ema *AlphaEMA) Update(observation float64, alpha float64) error {
	if alpha <= 0 || alpha > 1 {
		return fmt.Errorf("adaptive: AlphaEMA.Update alpha must be in (0,1], got %g", alpha)
	}

	ema.updates++

	if ema.updates == 1 {
		ema.value = observation

		return nil
	}

	ema.value += alpha * (observation - ema.value)

	return nil
}

/*
Value returns the current smoothed level. Before the first Update it is zero.
*/
func (ema *AlphaEMA) Value() float64 {
	return ema.value
}

/*
Updates counts how many observations have been absorbed.
*/
func (ema *AlphaEMA) Updates() int {
	return ema.updates
}

/*
Reset clears the tracker to its initial state.
*/
func (ema *AlphaEMA) Reset() {
	ema.value = 0
	ema.updates = 0
}
