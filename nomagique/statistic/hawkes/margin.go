package hawkes

import "errors"

/*
competitionMargin maps a positive excess-over-span pair into (0, 1): the
excess's share of the excess-plus-span total.
*/
func competitionMargin(excess, span float64) (float64, error) {
	if excess <= 0 || span <= 0 {
		return 0, errors.New("hawkes-margin: competition margin requires positive excess and span")
	}

	return excess / (excess + span), nil
}
