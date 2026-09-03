package types

// Split evaluates branches in parallel and computes their weighted sum: sum(w_i * Branch_i(x)).
// Empty slots degenerate to 0. Empty Route defaults to broadcast (w_i = 1).
type Split struct {
	Route      Router
	A, B, C, D Node
}

func (s *Split) Step(x Scalar) Scalar {
	wA, wB, wC, wD := Scalar(1), Scalar(1), Scalar(1), Scalar(1)

	if s.Route != nil {
		wA, wB, wC, wD = s.Route.Route(x)
	}

	var sum Scalar

	if s.A != nil && wA > 0 {
		sum += wA * s.A.Step(x)
	}

	if s.B != nil && wB > 0 {
		sum += wB * s.B.Step(x)
	}

	if s.C != nil && wC > 0 {
		sum += wC * s.C.Step(x)
	}

	if s.D != nil && wD > 0 {
		sum += wD * s.D.Step(x)
	}

	return sum
}
