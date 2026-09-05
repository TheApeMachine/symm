package learning

import "math"

/*
relax moves one coordinate under attraction and repulsion from the other
evidenced coordinates. Their target separation is min(||a-b||, ||a+b||) for
the accumulated signed profiles: consistent inverses attract, inconsistent
movement and relative magnitude differences increase separation.

The update minimizes a quadratic majorizer of weighted distance stress plus
the moving point's evidence-weighted squared displacement. Each neighbor
supplies its evidence as attraction/repulsion weight; the point's own evidence
resists displacement. No spring constant or learning rate is selected. For
fixed participating profile distances, this update cannot increase their
distance stress. An absent reading does not assert a conflicting movement.

Step cycles through one present point per input, comparing it with every other
present point. The cursor skips missing points to avoid coupling selection to
the frequency or order of different source updates.
This spreads optimization across the stream in linear work per input, without
an all-pairs table or iteration budget. It does not claim a global optimum or
instantaneous convergence while the incoming profiles themselves are changing.

The distance-stress majorization follows de Leeuw and Mair (2009):
https://www.jstatsoft.org/article/view/v031i03
*/
func (grid *Grid) relax(row int) {
	column := -1

	for range len(grid.Columns) {
		grid.cursor = (grid.cursor + 1) % len(grid.Columns)

		if grid.Present[row][grid.cursor] && grid.weights[grid.cursor] > 0 {
			column = grid.cursor
			break
		}
	}

	if column < 0 {
		return
	}

	weight := grid.weights[column]
	position := *grid.Coordinates[column]
	next := [2]float64{weight * position[0], weight * position[1]}
	countScale := math.Sqrt(float64(grid.Version))

	for peer, evidence := range grid.weights {
		if peer == column || evidence == 0 || !grid.Present[row][peer] {
			continue
		}

		product := grid.basis[0][column]*grid.basis[0][peer] +
			grid.basis[1][column]*grid.basis[1][peer]
		orientation := math.Copysign(1, product)
		profileHorizontal := (grid.basis[0][column] - orientation*grid.basis[0][peer]) / countScale
		profileVertical := (grid.basis[1][column] - orientation*grid.basis[1][peer]) / countScale
		target := math.Hypot(profileHorizontal, profileVertical)
		horizontal := position[0] - grid.Coordinates[peer][0]
		vertical := position[1] - grid.Coordinates[peer][1]
		distance := math.Hypot(horizontal, vertical)

		if distance == 0 {
			// At coincidence any unit direction majorizes the norm. The
			// measured profile difference supplies one without random jitter.
			horizontal, vertical, distance = profileHorizontal, profileVertical, target
		}

		if distance > 0 {
			horizontal *= target / distance
			vertical *= target / distance
		}

		next[0] += evidence * (grid.Coordinates[peer][0] + horizontal)
		next[1] += evidence * (grid.Coordinates[peer][1] + vertical)
		weight += evidence
	}

	*grid.Coordinates[column] = [2]float64{next[0] / weight, next[1] / weight}
}
