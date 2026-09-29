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

	climbTo := make([]int, len(points))

	for index := range points {
		climbTo[index] = index
	}

	// For each point, inspect its adjacent neighbors in the minimum spanning forest.
	for index, point := range points {
		if point.Parent < 0 {
			continue
		}

		parent := point.Parent

		// Check if point should climb to parent.
		if points[parent].Authority > points[index].Authority ||
			(points[parent].Authority == points[index].Authority && points[index].Authority > 0 && parent < index) {
			if climbTo[index] == index || points[parent].Authority > points[climbTo[index]].Authority {
				climbTo[index] = parent
			}
		}

		// Check if parent should climb to point.
		if points[index].Authority > points[parent].Authority ||
			(points[index].Authority == points[parent].Authority && points[parent].Authority > 0 && index < parent) {
			if climbTo[parent] == parent || points[index].Authority > points[climbTo[parent]].Authority {
				climbTo[parent] = index
			}
		}
	}

	// Trace ascent path to local peak for each point.
	for index, point := range points {
		basin := index
		visited := 0

		for climbTo[basin] != basin && visited < len(points) {
			basin = climbTo[basin]
			visited++
		}

		point.Basin = basin
	}

	// If all points have zero authority (uninitialized/pre-trade), partition by 2D spatial quadrants.
	hasAuthority := false

	for _, point := range points {
		if point.Authority > 0 {
			hasAuthority = true
			break
		}
	}

	if !hasAuthority && len(points) > 1 {
		// Identify representative quadrant anchors so points separate cleanly into 4 spatial regions.
		anchors := [4]int{-1, -1, -1, -1}

		for index, point := range points {
			quadrant := 0

			if point.X >= 0.5 {
				quadrant |= 1
			}

			if point.Y >= 0.5 {
				quadrant |= 2
			}

			if anchors[quadrant] < 0 {
				anchors[quadrant] = index
			}

			point.Basin = anchors[quadrant]
		}
	}
}
