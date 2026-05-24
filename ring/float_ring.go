package ring

/*
FloatRing is a fixed-capacity circular buffer for rolling float64 windows.
Push overwrites the oldest entry without heap allocation in the hot path.
*/
type FloatRing struct {
	values []float64
	head   int
	count  int
}

/*
NewFloatRing pre-allocates storage for capacity samples.
*/
func NewFloatRing(capacity int) FloatRing {
	if capacity <= 0 {
		capacity = 1
	}

	return FloatRing{values: make([]float64, capacity)}
}

/*
Push records one sample, evicting the oldest when full.
*/
func (ring *FloatRing) Push(value float64) {
	capacity := len(ring.values)
	ring.values[ring.head] = value
	ring.head = (ring.head + 1) % capacity

	if ring.count < capacity {
		ring.count++
	}
}

/*
Len returns the number of stored samples.
*/
func (ring FloatRing) Len() int {
	return ring.count
}

/*
At returns the sample at logical index 0 (oldest) through Len()-1 (newest).
*/
func (ring FloatRing) At(index int) float64 {
	if index < 0 || index >= ring.count {
		return 0
	}

	start := ring.startIndex()

	return ring.values[(start+index)%len(ring.values)]
}

/*
Last returns the most recent sample.
*/
func (ring FloatRing) Last() float64 {
	if ring.count == 0 {
		return 0
	}

	return ring.At(ring.count - 1)
}

/*
Ordered returns oldest-first samples. Allocates; use off the hot path only.
*/
func (ring FloatRing) Ordered() []float64 {
	if ring.count == 0 {
		return nil
	}

	ordered := make([]float64, ring.count)

	for index := 0; index < ring.count; index++ {
		ordered[index] = ring.At(index)
	}

	return ordered
}

func (ring FloatRing) startIndex() int {
	if ring.count < len(ring.values) {
		return 0
	}

	return ring.head
}
