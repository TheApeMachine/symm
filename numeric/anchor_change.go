package numeric

import "math"

/*
AnchorChange returns signed relative move and absolute magnitude from anchor to
current. Zero anchor yields zero move and magnitude.
*/
func AnchorChange(anchor, current float64) (move float64, magnitude float64) {
	if anchor <= 0 {
		return 0, 0
	}

	move = (current - anchor) / anchor
	magnitude = math.Abs(move)

	return move, magnitude
}
