package geometry

import "math"

/*
Motor combines rotation and translation into a single PGA versor.
Motors are the PGA equivalent of rigid-body transformations (SE(3));
the underlying Multivector lives entirely in the even subalgebra
so composition, interpolation, and sandwich products are closed.
*/
type Motor struct {
	rotor Multivector
}

/*
NewMotor wraps a pre-built even-subalgebra multivector as a Motor.
The caller is responsible for supplying a valid versor; no
normalization or validation is performed.
*/
func NewMotor(rotor Multivector) *Motor {
	return &Motor{rotor: rotor}
}

/*
MotorFromAxisAngle creates a rotation motor around an arbitrary axis.
The axis (axisX, axisY, axisZ) is normalized internally and mapped to
the dual bivector: x→e23, y→e31, z→e12 (Hodge dual of the rotation axis).
*/
func MotorFromAxisAngle(axisX, axisY, axisZ, angle float64) *Motor {
	norm := math.Sqrt(axisX*axisX + axisY*axisY + axisZ*axisZ)

	return NewMotor(Rotor(
		angle,
		axisZ/norm,
		axisY/norm,
		axisX/norm,
	))
}

/*
MotorFromTranslation creates a pure translation motor from a displacement
vector. The resulting motor is T = 1 + ½(dx·e01 + dy·e02 + dz·e03).
*/
func MotorFromTranslation(dx, dy, dz float64) *Motor {
	return NewMotor(Translator(dx, dy, dz))
}

/*
Apply transforms an even-grade multivector through this motor via the
sandwich product M · target · M†. Use for transforming bivectors, other
rotors, or any even-subalgebra element.
*/
func (motor *Motor) Apply(target Multivector) Multivector {
	return motor.rotor.Sandwich(target)
}

/*
Compose chains two motors so the receiver is applied first and other second:
result = other · motor. This preserves chronological ordering when reading
left to right in method chains.
*/
func (motor *Motor) Compose(other *Motor) *Motor {
	return NewMotor(other.rotor.GeometricProduct(motor.rotor))
}

/*
Interpolate performs fractional motor interpolation between the identity
(t=0) and the full motor (t=1). The Euclidean (rotation) part uses
angle-fraction SLERP via the half-angle decomposition; the ideal
(translation) part scales linearly. This is exact for pure rotors and
pure translators, and a first-order approximation for general motors.
*/
func (motor *Motor) Interpolate(t float64) *Motor {
	mv := motor.rotor

	eucNorm := math.Sqrt(
		mv[MvE12]*mv[MvE12] +
			mv[MvE31]*mv[MvE31] +
			mv[MvE23]*mv[MvE23],
	)

	halfAngle := math.Atan2(eucNorm, mv[MvScalar])

	if eucNorm < 1e-12 {
		return NewMotor(Multivector{
			1,
			t * mv[MvE01],
			t * mv[MvE02],
			t * mv[MvE03],
			0,
			0,
			0,
			0,
		})
	}

	newHalf := t * halfAngle
	sinRatio := math.Sin(newHalf) / eucNorm

	return NewMotor(Multivector{
		math.Cos(newHalf),
		t * mv[MvE01],
		t * mv[MvE02],
		t * mv[MvE03],
		sinRatio * mv[MvE12],
		sinRatio * mv[MvE31],
		sinRatio * mv[MvE23],
		t * mv[MvE0123],
	})
}

/*
Rotor returns the underlying Multivector of this motor.
*/
func (motor *Motor) Rotor() Multivector {
	return motor.rotor
}

/*
Point creates a PGA point from Euclidean coordinates.
In PGA a normalized point is the grade-3 trivector
P = e123 + x·e032 + y·e013 + z·e021.
Since grade-3 elements live outside the even subalgebra the four
components are returned as a plain [4]float64: [e123, e032, e013, e021].
*/
func Point(coordX, coordY, coordZ float64) [4]float64 {
	return [4]float64{1, coordX, coordY, coordZ}
}
