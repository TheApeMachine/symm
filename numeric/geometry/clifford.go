package geometry

import "math"

/*
Multivector represents a grade-restricted element of PGA Cl(3,0,1).
Only the even subalgebra (rotor-viable grades 0 and 2, plus pseudoscalar)
is stored, giving 8 float64 components instead of the full 16.

Layout: [scalar, e01, e02, e03, e12, e31, e23, e0123]

Grade 0 (scalar) and grade 2 (bivectors) carry rotation and translation;
grade 4 (pseudoscalar e0123) closes the subalgebra. The degenerate basis
vector e0 (e0²=0) distinguishes ideal (translation) bivectors e01,e02,e03
from Euclidean (rotation) bivectors e12,e31,e23.
*/
type Multivector [8]float64

const (
	MvScalar = iota
	MvE01
	MvE02
	MvE03
	MvE12
	MvE31
	MvE23
	MvE0123
)

/*
Rotor constructs a rotation rotor from an axis bivector and angle.
The axis components (axisE12, axisE31, axisE23) should form a unit bivector;
no internal normalization is applied. The result encodes cos(θ/2) + sin(θ/2)·B
where B = axisE12·e12 + axisE31·e31 + axisE23·e23.
*/
func Rotor(angle float64, axisE12, axisE31, axisE23 float64) Multivector {
	half := angle / 2
	sinHalf := math.Sin(half)

	return Multivector{
		math.Cos(half),
		0,
		0,
		0,
		sinHalf * axisE12,
		sinHalf * axisE31,
		sinHalf * axisE23,
		0,
	}
}

/*
Translator constructs a translation motor from a displacement vector.
T = 1 + ½(dx·e01 + dy·e02 + dz·e03). The ideal bivectors e0i square to
zero, so T·T† = 1 exactly and the sandwich product shifts points by (dx,dy,dz).
*/
func Translator(dx, dy, dz float64) Multivector {
	return Multivector{
		1,
		dx / 2,
		dy / 2,
		dz / 2,
		0,
		0,
		0,
		0,
	}
}

/*
GeometricProduct computes the PGA geometric product of two even-subalgebra
multivectors using the Cayley table for Cl(3,0,1) with signature
e0²=0, e1²=e2²=e3²=1. Each output component is a sparse linear combination
of input pairs; the 8×8 table was derived by bubble-sorting basis vector
sequences and contracting via the metric.
*/
func (mv Multivector) GeometricProduct(other Multivector) Multivector {
	return Multivector{
		mv[0]*other[0] - mv[4]*other[4] - mv[5]*other[5] - mv[6]*other[6],

		mv[0]*other[1] + mv[1]*other[0] - mv[2]*other[4] + mv[3]*other[5] +
			mv[4]*other[2] - mv[5]*other[3] - mv[6]*other[7] - mv[7]*other[6],

		mv[0]*other[2] + mv[1]*other[4] + mv[2]*other[0] - mv[3]*other[6] -
			mv[4]*other[1] - mv[5]*other[7] + mv[6]*other[3] - mv[7]*other[5],

		mv[0]*other[3] - mv[1]*other[5] + mv[2]*other[6] + mv[3]*other[0] -
			mv[4]*other[7] + mv[5]*other[1] - mv[6]*other[2] - mv[7]*other[4],

		mv[0]*other[4] + mv[4]*other[0] + mv[5]*other[6] - mv[6]*other[5],

		mv[0]*other[5] - mv[4]*other[6] + mv[5]*other[0] + mv[6]*other[4],

		mv[0]*other[6] + mv[4]*other[5] - mv[5]*other[4] + mv[6]*other[0],

		mv[0]*other[7] + mv[1]*other[6] + mv[2]*other[5] + mv[3]*other[4] +
			mv[4]*other[3] + mv[5]*other[2] + mv[6]*other[1] + mv[7]*other[0],
	}
}

/*
Reverse computes the reverse (†) of a multivector: reverses the ordering of
basis vectors in each blade. For grade k the sign is (-1)^(k(k-1)/2), giving
+1 for grades 0 and 4, and -1 for grade 2. This negates all six bivector
components while preserving the scalar and pseudoscalar.
*/
func (mv Multivector) Reverse() Multivector {
	return Multivector{
		mv[MvScalar],
		-mv[MvE01],
		-mv[MvE02],
		-mv[MvE03],
		-mv[MvE12],
		-mv[MvE31],
		-mv[MvE23],
		mv[MvE0123],
	}
}

/*
Sandwich computes the sandwich product mv · target · mv† which applies the
rigid transformation encoded by mv to target. Both operands must live in the
even subalgebra; the product is closed.
*/
func (mv Multivector) Sandwich(target Multivector) Multivector {
	return mv.GeometricProduct(target).GeometricProduct(mv.Reverse())
}

/*
Normalize scales the multivector so its Euclidean bulk norm (the part that
generates rotations) is unity: √(s² + e12² + e31² + e23²) = 1. All eight
components are divided by this norm. If the bulk is zero the multivector is
returned unchanged — that case signals a degenerate (pure-ideal) element.
*/
func (mv Multivector) Normalize() Multivector {
	bulkSq := mv[MvScalar]*mv[MvScalar] +
		mv[MvE12]*mv[MvE12] +
		mv[MvE31]*mv[MvE31] +
		mv[MvE23]*mv[MvE23]

	if bulkSq == 0 {
		return mv
	}

	inv := 1.0 / math.Sqrt(bulkSq)

	return Multivector{
		mv[0] * inv,
		mv[1] * inv,
		mv[2] * inv,
		mv[3] * inv,
		mv[4] * inv,
		mv[5] * inv,
		mv[6] * inv,
		mv[7] * inv,
	}
}

/*
Compose chains two rotors so the receiver is applied first and other second:
result = other · mv. This follows the standard convention where right-to-left
multiplication order matches chronological application order.
*/
func (mv Multivector) Compose(other Multivector) Multivector {
	return other.GeometricProduct(mv)
}
