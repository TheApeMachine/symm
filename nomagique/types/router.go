package types

// Router evaluates an incoming signal and emits 4 branch weights for parallel routing.
type Router interface {
	Route(Scalar) (Scalar, Scalar, Scalar, Scalar)
}
