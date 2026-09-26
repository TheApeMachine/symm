/*
	Package dependence supplies interval mathematics shared by the graph's

explicit relation store and stateless lag-search node. It owns no retained
market state, transport or execution loop.
*/
package dependence

import (
	"math"
	"sort"
)

/* Point is one positive observed price at a nanosecond timestamp. */
type Point struct {
	At    int64
	Value float64
}

/* Interval carries a log return over its actual observation interval. */
type Interval struct {
	From, To int64
	Value    float64
}

/* Path derives returns and their energy once for a retained price history. */
type Path struct {
	Points                []Point
	Returns               []Interval
	Energy, Rate, Spacing float64
}

/* Measure derives interval facts without inventing synchronized returns. */
func (path *Path) Measure() {
	path.Returns = path.Returns[:0]
	path.Energy, path.Rate, path.Spacing = 0, 0, 0
	rates := make([]float64, 0, len(path.Points))
	spacings := make([]float64, 0, len(path.Points))
	for index := 1; index < len(path.Points); index++ {
		previous, current := path.Points[index-1], path.Points[index]
		value := math.Log(current.Value / previous.Value)
		path.Returns = append(path.Returns, Interval{previous.At, current.At, value})
		energy := value * value
		path.Energy += energy
		duration := float64(current.At - previous.At)
		rates = append(rates, energy/(duration/1e9))
		spacings = append(spacings, duration)
	}

	if len(rates) == 0 {
		return
	}
	sort.Float64s(rates)
	sort.Float64s(spacings)
	count := len(rates)
	path.Rate = (rates[(count-1)/2] + rates[count/2]) / 2
	path.Spacing = (spacings[(count-1)/2] + spacings[count/2]) / 2
}

/* Estimate is the unresampled Hayashi-Yoshida pair reading. */
type Estimate struct {
	Correlation, Covariance, Support, SharedTime float64
	Defined                                      bool
}

/*
	Compare shifts the left clock by lag nanoseconds and sums strict overlaps.

Both path energies include every retained return exactly once, as in LEGACY.
*/
func (path *Path) Compare(right *Path, lag int64) Estimate {
	result := Estimate{}
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(path.Returns) && rightIndex < len(right.Returns) {
		leftReturn, rightReturn := path.Returns[leftIndex], right.Returns[rightIndex]
		leftFrom, leftTo := leftReturn.From+lag, leftReturn.To+lag

		if leftFrom < rightReturn.To && rightReturn.From < leftTo {
			result.Covariance += leftReturn.Value * rightReturn.Value
			result.Support++
		}

		if leftTo <= rightReturn.To {
			leftIndex++
			continue
		}
		rightIndex++
	}

	if result.Support == 0 || path.Energy <= 0 || right.Energy <= 0 {
		return result
	}
	result.Correlation = result.Covariance / math.Sqrt(path.Energy*right.Energy)
	result.Defined = true
	start := max(path.Points[0].At+lag, right.Points[0].At)
	end := min(path.Points[len(path.Points)-1].At+lag, right.Points[len(right.Points)-1].At)

	if end > start {
		result.SharedTime = float64(end-start) / 1e9
	}
	return result
}
