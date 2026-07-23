package fluid

import "fmt"

/*
fluxAccumulator measures book churn and fill volume over volume-clocked bars.

Bars close once a configured base-volume target has traded, so each bucket carries
a consistent quantum of activity. Book churn accumulates continuously within the
open bar and is paired with trade volume when the bar closes.
*/
type fluxAccumulator struct {
	target   float64
	progress float64
}

func newFluxAccumulator() *fluxAccumulator {
	return &fluxAccumulator{}
}

/*
setTarget sets the base volume that closes one bar.
*/
func (flux *fluxAccumulator) setTarget(target float64) error {
	if target <= 0 {
		return fmt.Errorf("fluid: volume clock bar target must be positive, got %v", target)
	}

	flux.target = target

	return nil
}

/*
hasTarget reports whether a conservation target exists before flux is
evaluated.
*/
func (flux *fluxAccumulator) hasTarget() bool {
	return flux.target > 0
}

/*
addBook folds one book-churn reading into the open volume bar.
*/
func (flux *fluxAccumulator) addBook(churn float64) error {
	if churn <= 0 {
		return nil
	}

	if flux.target <= 0 {
		return fmt.Errorf("fluid: volume clock target is not set")
	}

	return nil
}

/*
addTrade folds one fill into the open bar and closes it once the volume target is met.
*/
func (flux *fluxAccumulator) addTrade(qty float64) error {
	if qty <= 0 {
		return nil
	}

	if flux.target <= 0 {
		return fmt.Errorf("fluid: volume clock target is not set")
	}

	flux.progress += qty

	if flux.progress >= flux.target {
		flux.close()
	}

	return nil
}

/*
close clears completed bar totals while retaining the configured volume target
so the next bar uses the same empirically established scale.
*/
func (flux *fluxAccumulator) close() {
	flux.progress = 0
}
