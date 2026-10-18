package tokenizer

import "math"

/*
FastWindow replaces container/ring with an O(1) array-backed sliding window.
It includes anti-drift recalibration to prevent floating-point errors over long streams.
*/
type FastWindow struct {
	data   []float64
	size   int
	count  int
	head   int
	sum    float64
	sumSq  float64
	drifts int
}

/*
NewFastWindow allocates a sliding window of the given size. O(1) push; recalc every size*2 pushes.
*/
func NewFastWindow(size int) *FastWindow {
	if size <= 0 {
		return nil
	}
	
	return &FastWindow{
		data: make([]float64, size),
		size: size,
	}
}

/*
Push adds val to the window, evicting the oldest if full. Triggers recalc to prevent float drift.
*/
func (window *FastWindow) Push(val float64) {
	if window.count == window.size {
		old := window.data[window.head]
		window.sum -= old
		window.sumSq -= old * old
	} else {
		window.count++
	}

	window.data[window.head] = val
	window.sum += val
	window.sumSq += val * val
	window.head = (window.head + 1) % window.size

	// Prevent floating-point drift over millions of bytes
	window.drifts++

	if window.drifts >= window.size*2 {
		window.recalc()
	}
}

/*
recalc recomputes sum and sumSq from scratch over all window entries.
Called periodically (every size*2 pushes) to prevent floating-point drift
in incremental updates over long streams.
*/
func (window *FastWindow) recalc() {
	sum := 0.0
	sumSq := 0.0
	
	for idx := 0; idx < window.count; idx++ {
		entry := window.data[idx]
		sum += entry
		sumSq += entry * entry
	}
	
	window.sum = sum
	window.sumSq = sumSq
	window.drifts = 0
}

/*
SimulatePush returns mean and stddev as if val were pushed without modifying the window.
*/
func (window *FastWindow) SimulatePush(val float64) (mean, stddev float64) {
	simSum := window.sum + val
	simSumSq := window.sumSq + (val * val)
	simCount := window.count

	if window.count == window.size {
		old := window.data[window.head]
		simSum -= old
		simSumSq -= old * old
	} else {
		simCount++
	}

	count := float64(simCount)
	mean = simSum / count
	
	if simCount < 2 {
		return mean, 0
	}

	variance := (simSumSq - (simSum*simSum)/count) / float64(simCount-1)
	
	if variance > 0 {
		stddev = math.Sqrt(variance)
	}
	
	return mean, stddev
}

/*
Warmed returns true when the window has received at least size values.
*/
func (window *FastWindow) Warmed() bool {
	return window.count >= window.size
}

/*
Stats returns the current mean and sample stddev of the window contents.
*/
func (window *FastWindow) Stats() (mean, stddev float64) {
	if window.count == 0 {
		return 0, 0
	}

	count := float64(window.count)
	mean = window.sum / count

	if window.count < 2 {
		return mean, 0
	}

	variance := (window.sumSq - (window.sum*window.sum)/count) / float64(window.count-1)
	
	if variance > 0 {
		stddev = math.Sqrt(variance)
	}
	
	return
}
