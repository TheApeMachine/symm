package adaptive

import (
	"fmt"
	"time"
)

/*
Window accumulates samples over a fixed duration, returning the closed sum when
the window rolls and the running sum while the window is open.
*/
type Window struct {
	width  time.Duration
	start  time.Time
	sum    float64
	anchor float64
}

/*
NewWindow creates a timed accumulation window.
*/
func NewWindow(width time.Duration) *Window {
	return &Window{width: width}
}

/*
Next expects unix nanoseconds and a sample. An optional third value captures
the reference level at window open (for example price at bucket start).
When the window rolls it returns the closed sum; otherwise the running sum.
*/
func (window *Window) Next(_ float64, values ...float64) (float64, error) {
	if len(values) < 2 {
		return 0, fmt.Errorf("adaptive: Window.Next expects unix nanos and sample")
	}

	at := time.Unix(0, int64(values[0]))
	sample := values[1]

	anchor := 0.0

	if len(values) >= 3 {
		anchor = values[2]
	}

	if window.start.IsZero() {
		window.start = at
		window.sum = sample

		if anchor > 0 {
			window.anchor = anchor
		}

		return window.sum, nil
	}

	if at.Sub(window.start) >= window.width {
		closed := window.sum
		window.start = at
		window.sum = sample

		if anchor > 0 {
			window.anchor = anchor
		}

		return closed, nil
	}

	window.sum += sample

	return window.sum, nil
}

/*
Sum returns the running total for the open window.
*/
func (window *Window) Sum() float64 {
	return window.sum
}

/*
Anchor returns the reference level recorded at window open.
*/
func (window *Window) Anchor() float64 {
	return window.anchor
}

/*
Reset clears window state.
*/
func (window *Window) Reset() error {
	window.start = time.Time{}
	window.sum = 0
	window.anchor = 0

	return nil
}
