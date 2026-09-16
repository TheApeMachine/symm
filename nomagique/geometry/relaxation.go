package geometry

import "math"

/* Point is an addressable coordinate and its measured resistance to movement. */
type Point struct {
	X, Y               float64
	Authority          float64
	MoveX, MoveY, Mass float64
	Parent, Basin      int
	Distance           float64
	Visited            bool
}

/* Edge expresses signed attraction between two point indices. */
type Edge struct {
	Left, Right int
	Strength    float64
}

/*
Relaxation performs one simultaneous weighted stress descent. Unit distance is
the coordinate system's unit, not a market threshold. Positive evidence lowers
target distance; negative evidence increases it. Endpoint authority determines
the opposite endpoint's share of displacement. Scratch fields live with points.
*/
type Relaxation struct{}

func (relaxation Relaxation) Step(points []*Point, edges []Edge) {
	for _, point := range points {
		point.MoveX, point.MoveY, point.Mass = 0, 0, 0
	}

	for _, edge := range edges {
		left, right := points[edge.Left], points[edge.Right]
		mass := left.Authority + right.Authority

		if mass == 0 || edge.Strength == 0 {
			continue
		}

		deltaX, deltaY := right.X-left.X, right.Y-left.Y
		distance := math.Hypot(deltaX, deltaY)

		if distance == 0 {
			continue
		}

		target := 1 - edge.Strength

		if edge.Strength > 0 {
			target = 1 / (1 + edge.Strength)
		}

		weight := math.Abs(edge.Strength)
		force := weight * (distance - target) / distance
		left.MoveX += force * deltaX * right.Authority / mass
		left.MoveY += force * deltaY * right.Authority / mass
		right.MoveX -= force * deltaX * left.Authority / mass
		right.MoveY -= force * deltaY * left.Authority / mass
		left.Mass += weight
		right.Mass += weight
	}

	for _, point := range points {
		if point.Mass > 0 {
			point.X += point.MoveX / point.Mass
			point.Y += point.MoveY / point.Mass
		}
	}
}
