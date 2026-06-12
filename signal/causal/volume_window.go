package causal

import (
	"fmt"
	"time"
)

/*
VolumeWindow accumulates trade volume over a fixed duration, returning the closed
sum when the window rolls and the running sum while the window is open.
*/
type VolumeWindow struct {
	width  time.Duration
	start  time.Time
	sum    float64
	anchor float64
}

func NewVolumeWindow(width time.Duration) *VolumeWindow {
	return &VolumeWindow{width: width}
}

func (volumeWindow *VolumeWindow) Next(_ float64, values ...float64) (float64, error) {
	if len(values) < 2 {
		return 0, fmt.Errorf("causal: VolumeWindow.Next expects unix nanos and sample")
	}

	at := time.Unix(0, int64(values[0]))
	sample := values[1]

	anchor := 0.0

	if len(values) >= 3 {
		anchor = values[2]
	}

	if volumeWindow.start.IsZero() {
		volumeWindow.start = at
		volumeWindow.sum = sample

		if anchor > 0 {
			volumeWindow.anchor = anchor
		}

		return volumeWindow.sum, nil
	}

	if at.Sub(volumeWindow.start) >= volumeWindow.width {
		closed := volumeWindow.sum
		volumeWindow.start = at
		volumeWindow.sum = sample

		if anchor > 0 {
			volumeWindow.anchor = anchor
		}

		return closed, nil
	}

	volumeWindow.sum += sample

	return volumeWindow.sum, nil
}

func (volumeWindow *VolumeWindow) Sum() float64 {
	return volumeWindow.sum
}

func (volumeWindow *VolumeWindow) Anchor() float64 {
	return volumeWindow.anchor
}

func (volumeWindow *VolumeWindow) Reset() error {
	volumeWindow.start = time.Time{}
	volumeWindow.sum = 0
	volumeWindow.anchor = 0

	return nil
}
