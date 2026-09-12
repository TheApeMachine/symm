package grid

import "math"

/*
form calibrates one fixed affinity objective from the retained feature window,
then advances one coordinate per observed quantity until a complete sweep cannot reduce
the represented distance stress. This is numerical
convergence of a fixed objective, not a claim that the market stopped changing.
Ordinary observations cannot restart a completed formation.
*/
func (grid *Space) form() {
	if grid.formed || grid.window.count < grid.window.capacity {
		return
	}

	if grid.graph == nil {
		if !grid.calibrate() {
			return
		}
	}

	grid.relax()

	if grid.cursor != len(grid.columns)-1 {
		return
	}

	if grid.moved {
		grid.moved = false
		return
	}

	grid.regions.form(grid)
	grid.formed = true
}

/*
calibrate fixes symmetric relationships and evidence weights for formation.
An unobserved relationship stays absent. At least one evidenced pair is needed
before a layout can claim to have balanced anything.
*/
func (grid *Space) calibrate() bool {
	graph := make([][]affinity, len(grid.columns))

	for column := range graph {
		graph[column] = make([]affinity, len(graph))
	}
	pairs := 0

	for left := range graph {
		for right := left + 1; right < len(graph); right++ {
			reading := grid.window.measure(left, right)

			if reading.shared < 2 || reading.strength() == 0 ||
				grid.weights[left] == 0 || grid.weights[right] == 0 {
				continue
			}
			graph[left][right], graph[right][left] = reading, reading
			pairs++
		}
	}

	if pairs == 0 {
		return false
	}
	grid.graph = graph
	grid.cursor, grid.moved = -1, false
	return true
}

/*
relax minimizes the existing weighted distance-stress majorizer, with the
point's own evidence resisting displacement. The calibrated graph includes
relationships between producers arriving in different envelopes.

The objective is fixed throughout this solve. This is required by the
majorization argument; changing the affinity matrix on every iteration does
not establish convergence. See de Leeuw and Mair (2009):
https://www.jstatsoft.org/article/view/v031i03
*/
func (grid *Space) relax() {
	grid.cursor = (grid.cursor + 1) % len(grid.columns)
	column := grid.cursor
	weight := grid.weights[column]

	if weight == 0 {
		return
	}
	position := *grid.coordinates[column]
	next := [2]float64{weight * position[0], weight * position[1]}

	for peer, evidence := range grid.weights {
		reading := grid.graph[column][peer]

		if peer == column || evidence == 0 || reading.shared < 2 {
			continue
		}
		pull := evidence * reading.strength()

		if pull <= 0 {
			continue
		}
		target := grid.separation(reading)
		horizontal := position[0] - grid.coordinates[peer][0]
		vertical := position[1] - grid.coordinates[peer][1]
		distance := math.Hypot(horizontal, vertical)

		if distance == 0 {
			// A deterministic direction breaks coincidence without collapsing
			// every repulsive pair onto the horizontal axis. The angle names
			// the peer on the unit circle; it is not a learning parameter.
			angle := 2 * math.Pi * float64(peer) / float64(len(grid.columns))
			horizontal, vertical, distance = math.Cos(angle), math.Sin(angle), 1
		}
		next[0] += pull * (grid.coordinates[peer][0] + horizontal*target/distance)
		next[1] += pull * (grid.coordinates[peer][1] + vertical*target/distance)
		weight += pull
	}

	for dimension := range next {
		next[dimension] /= weight
	}

	// Rounding can move a coordinate without improving the fixed objective.
	// Reject that move rather than waiting forever for coordinates to stop
	// jittering, or declaring convergence after a chosen iteration budget.
	if grid.stress(column, next) >= grid.stress(column, position) {
		return
	}
	*grid.coordinates[column] = next
	grid.moved = true
}

/* stress measures the incident objective; the node's common weight cancels. */
func (grid *Space) stress(column int, position [2]float64) float64 {
	total := 0.0

	for peer, reading := range grid.graph[column] {
		if peer == column || reading.shared < 2 {
			continue
		}
		distance := math.Hypot(position[0]-grid.coordinates[peer][0],
			position[1]-grid.coordinates[peer][1])
		residual := distance - grid.separation(reading)
		total += grid.weights[peer] * reading.strength() * residual * residual
	}
	return total
}

/* separation maps stable relationships to attraction and instability to repulsion. */
func (grid *Space) separation(reading affinity) float64 {
	strength := math.Min(reading.strength(), 1)

	if reading.stable() {
		return 1 - strength
	}

	return 1 + strength
}
