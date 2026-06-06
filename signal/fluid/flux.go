package fluid

import "fmt"

/*
fluxAccumulator measures book churn and fill volume over volume-clocked bars.

Bars close once a configured base-volume target has traded, so each bucket carries
a consistent quantum of activity. Book churn is counted only while the open bar
already has trade volume — quiet book wiggles outside an active volume bar are
ignored rather than substituted with a wall-clock window.
*/
type fluxAccumulator struct {
	target      float64
	progress    float64
	bookOpen    float64
	tradeOpen   float64
	bookClosed  float64
	tradeClosed float64
	haveClosed  bool
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
addBook folds one book-churn reading into the open volume bar. Churn is ignored
until the bar has trade volume, because a fill-to-cancel ratio without fills is
undefined.
*/
func (flux *fluxAccumulator) addBook(churn float64) error {
	if churn <= 0 {
		return nil
	}

	if flux.target <= 0 {
		return fmt.Errorf("fluid: volume clock target is not set")
	}

	if flux.tradeOpen <= 0 {
		return nil
	}

	flux.bookOpen += churn

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

	flux.tradeOpen += qty
	flux.progress += qty

	if flux.progress >= flux.target {
		flux.close()
	}

	return nil
}

func (flux *fluxAccumulator) close() {
	flux.bookClosed = flux.bookOpen
	flux.tradeClosed = flux.tradeOpen
	flux.haveClosed = true
	flux.bookOpen = 0
	flux.tradeOpen = 0
	flux.progress = 0
}

/*
completedBar returns the last volume-closed bar. Open or partial bars are not
substituted.
*/
func (flux *fluxAccumulator) completedBar() (bookFlux, tradeFlux float64, err error) {
	if !flux.haveClosed {
		return 0, 0, fmt.Errorf("fluid: flux bar has not completed")
	}

	if flux.tradeClosed <= 0 {
		return 0, 0, fmt.Errorf("fluid: completed flux bar has no trade volume")
	}

	return flux.bookClosed, flux.tradeClosed, nil
}
