package types

// Identity is the transparent identity pass-through: I(x) = x.
type Identity struct{}

func (Identity) Step(x Scalar) Scalar {
	return x
}

type IdentityNode = Identity
