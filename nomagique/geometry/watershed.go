package geometry

import "math"

/*
Watershed finds density peaks on the Euclidean minimum spanning forest of
positively related points. Prim's traversal and plateau ties follow point
order. Edges use lower-triangular order: right*(right-1)/2+left. Each point climbs to its strongest adjacent higher-authority point;
edges joining different peaks are the weak meeting borders. No region count,
neighbour count, or radius is selected.
*/
type Watershed struct{}

func (watershed Watershed) Step(points []*Point, edges []Edge) {
	for index, point := range points {
		point.Parent, point.Basin = -1, index
		point.Distance = math.Inf(1)
		point.Visited = false
	}

	for range points {
		selected := -1

		for index, point := range points {
			if !point.Visited && (selected < 0 || point.Distance < points[selected].Distance) {
				selected = index
			}
		}

		points[selected].Visited = true

		for peer, point := range points {
			if point.Visited {
				continue
			}

			left, right := min(selected, peer), max(selected, peer)

			if edges[right*(right-1)/2+left].Strength <= 0 {
				continue
			}

			deltaX, deltaY := points[selected].X-point.X, points[selected].Y-point.Y
			distance := deltaX*deltaX + deltaY*deltaY

			if distance < point.Distance {
				point.Distance, point.Parent = distance, selected
			}
		}
	}

	for index, point := range points {
		if point.Parent >= 0 {
			watershed.climb(points, index, point.Parent)
			watershed.climb(points, point.Parent, index)
		}
	}

	for _, point := range points {
		for point.Basin != points[point.Basin].Basin {
			point.Basin = points[point.Basin].Basin
		}
	}
}

func (watershed Watershed) climb(points []*Point, index, peer int) {
	current := points[index].Basin

	if points[peer].Authority > points[current].Authority ||
		(points[peer].Authority == points[current].Authority && peer < current) {
		points[index].Basin = peer
	}
}
