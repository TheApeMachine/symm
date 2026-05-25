package geometry

import (
	"math"
	"testing"

	gc "github.com/smartystreets/goconvey/convey"
)

func TestMotorFromAxisAngle(t *testing.T) {
	gc.Convey("Given MotorFromAxisAngle", t, func() {
		gc.Convey("When creating a 90° rotation around the Z axis", func() {
			motor := MotorFromAxisAngle(0, 0, 1, math.Pi/2)
			mv := motor.Rotor()

			gc.Convey("It should encode cos(π/4) scalar and sin(π/4) in e12", func() {
				gc.So(mv[MvScalar], gc.ShouldAlmostEqual, math.Cos(math.Pi/4), 1e-12)
				gc.So(mv[MvE12], gc.ShouldAlmostEqual, math.Sin(math.Pi/4), 1e-12)
				gc.So(math.Abs(mv[MvE31]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(mv[MvE23]), gc.ShouldBeLessThan, 1e-12)
			})
		})

		gc.Convey("When creating a 90° rotation around the X axis", func() {
			motor := MotorFromAxisAngle(1, 0, 0, math.Pi/2)
			mv := motor.Rotor()

			gc.Convey("It should place sin(π/4) in e23 (Hodge dual of X)", func() {
				gc.So(mv[MvScalar], gc.ShouldAlmostEqual, math.Cos(math.Pi/4), 1e-12)
				gc.So(mv[MvE23], gc.ShouldAlmostEqual, math.Sin(math.Pi/4), 1e-12)
				gc.So(math.Abs(mv[MvE12]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(mv[MvE31]), gc.ShouldBeLessThan, 1e-12)
			})
		})

		gc.Convey("When creating a 90° rotation around the Y axis", func() {
			motor := MotorFromAxisAngle(0, 1, 0, math.Pi/2)
			mv := motor.Rotor()

			gc.Convey("It should place sin(π/4) in e31 (Hodge dual of Y)", func() {
				gc.So(mv[MvScalar], gc.ShouldAlmostEqual, math.Cos(math.Pi/4), 1e-12)
				gc.So(mv[MvE31], gc.ShouldAlmostEqual, math.Sin(math.Pi/4), 1e-12)
				gc.So(math.Abs(mv[MvE12]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(mv[MvE23]), gc.ShouldBeLessThan, 1e-12)
			})
		})
	})
}

func TestMotorFromTranslation(t *testing.T) {
	gc.Convey("Given MotorFromTranslation", t, func() {
		gc.Convey("When creating a translation by (3, 5, 7)", func() {
			motor := MotorFromTranslation(3, 5, 7)
			mv := motor.Rotor()

			gc.Convey("It should store half-displacements in ideal bivectors", func() {
				gc.So(mv[MvScalar], gc.ShouldEqual, 1.0)
				gc.So(mv[MvE01], gc.ShouldEqual, 1.5)
				gc.So(mv[MvE02], gc.ShouldEqual, 2.5)
				gc.So(mv[MvE03], gc.ShouldEqual, 3.5)
			})

			gc.Convey("It should have zero Euclidean bivector content", func() {
				gc.So(mv[MvE12], gc.ShouldEqual, 0.0)
				gc.So(mv[MvE31], gc.ShouldEqual, 0.0)
				gc.So(mv[MvE23], gc.ShouldEqual, 0.0)
			})
		})
	})
}

func TestMotorCompose(t *testing.T) {
	gc.Convey("Given two rotation motors", t, func() {
		gc.Convey("When composing two 90° Z-rotations", func() {
			rot90 := MotorFromAxisAngle(0, 0, 1, math.Pi/2)
			composed := rot90.Compose(rot90)
			direct180 := MotorFromAxisAngle(0, 0, 1, math.Pi)

			gc.Convey("It should equal a single 180° rotation", func() {
				composedMv := composed.Rotor()
				directMv := direct180.Rotor()

				for idx := range 8 {
					gc.So(composedMv[idx], gc.ShouldAlmostEqual, directMv[idx], 1e-12)
				}
			})
		})

		gc.Convey("When composing rotation with identity", func() {
			rot := MotorFromAxisAngle(1, 0, 0, 1.23)
			identity := NewMotor(Multivector{1, 0, 0, 0, 0, 0, 0, 0})
			composed := rot.Compose(identity)

			gc.Convey("It should return the original rotation", func() {
				rotMv := rot.Rotor()
				composedMv := composed.Rotor()

				for idx := range 8 {
					gc.So(composedMv[idx], gc.ShouldAlmostEqual, rotMv[idx], 1e-12)
				}
			})
		})
	})
}

func TestMotorInterpolate(t *testing.T) {
	gc.Convey("Given a 90° rotation motor around Z", t, func() {
		motor := MotorFromAxisAngle(0, 0, 1, math.Pi/2)

		gc.Convey("When interpolated at t=0", func() {
			interp := motor.Interpolate(0)
			mv := interp.Rotor()

			gc.Convey("It should give the identity motor", func() {
				gc.So(mv[MvScalar], gc.ShouldAlmostEqual, 1.0, 1e-12)
				gc.So(math.Abs(mv[MvE12]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(mv[MvE31]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(mv[MvE23]), gc.ShouldBeLessThan, 1e-12)
			})
		})

		gc.Convey("When interpolated at t=1", func() {
			interp := motor.Interpolate(1)
			interpMv := interp.Rotor()
			originalMv := motor.Rotor()

			gc.Convey("It should give the full motor", func() {
				for idx := range 8 {
					gc.So(interpMv[idx], gc.ShouldAlmostEqual, originalMv[idx], 1e-12)
				}
			})
		})

		gc.Convey("When interpolated at t=0.5", func() {
			interp := motor.Interpolate(0.5)
			mv := interp.Rotor()

			gc.Convey("It should give a 45° rotation (half of 90°)", func() {
				gc.So(mv[MvScalar], gc.ShouldAlmostEqual, math.Cos(math.Pi/8), 1e-12)
				gc.So(mv[MvE12], gc.ShouldAlmostEqual, math.Sin(math.Pi/8), 1e-12)
			})
		})
	})

	gc.Convey("Given a pure translation motor", t, func() {
		motor := MotorFromTranslation(4, 6, 8)

		gc.Convey("When interpolated at t=0.5", func() {
			interp := motor.Interpolate(0.5)
			mv := interp.Rotor()

			gc.Convey("It should give half the translation", func() {
				gc.So(mv[MvScalar], gc.ShouldAlmostEqual, 1.0, 1e-12)
				gc.So(mv[MvE01], gc.ShouldAlmostEqual, 1.0, 1e-12)
				gc.So(mv[MvE02], gc.ShouldAlmostEqual, 1.5, 1e-12)
				gc.So(mv[MvE03], gc.ShouldAlmostEqual, 2.0, 1e-12)
			})
		})
	})
}

func TestMotorApply(t *testing.T) {
	gc.Convey("Given a 90° Z-rotation motor", t, func() {
		motor := MotorFromAxisAngle(0, 0, 1, math.Pi/2)
		target := Multivector{0, 0, 0, 0, 0, 1, 0, 0}

		gc.Convey("When applying to e31 bivector", func() {
			result := motor.Apply(target)

			gc.Convey("It should rotate e31 → +e23 (e3∧e1 → e3∧(-e2) = +e23)", func() {
				gc.So(result[MvE23], gc.ShouldAlmostEqual, 1.0, 1e-12)
				gc.So(math.Abs(result[MvE31]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(result[MvE12]), gc.ShouldBeLessThan, 1e-12)
			})
		})
	})
}

func TestPoint(t *testing.T) {
	gc.Convey("Given the Point constructor", t, func() {
		gc.Convey("When creating a point at (3, 5, 7)", func() {
			pt := Point(3, 5, 7)

			gc.Convey("It should store [1, x, y, z] as trivector components", func() {
				gc.So(pt[0], gc.ShouldEqual, 1.0)
				gc.So(pt[1], gc.ShouldEqual, 3.0)
				gc.So(pt[2], gc.ShouldEqual, 5.0)
				gc.So(pt[3], gc.ShouldEqual, 7.0)
			})
		})

		gc.Convey("When creating the origin", func() {
			pt := Point(0, 0, 0)

			gc.Convey("It should be the unit pseudoscalar e123", func() {
				gc.So(pt[0], gc.ShouldEqual, 1.0)
				gc.So(pt[1], gc.ShouldEqual, 0.0)
				gc.So(pt[2], gc.ShouldEqual, 0.0)
				gc.So(pt[3], gc.ShouldEqual, 0.0)
			})
		})
	})
}

func BenchmarkMotorFromAxisAngle(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_ = MotorFromAxisAngle(0.577, 0.577, 0.577, 1.23)
	}
}

func BenchmarkMotorApply(b *testing.B) {
	motor := MotorFromAxisAngle(0.577, 0.577, 0.577, 1.23)
	target := Multivector{0, 0, 0, 0, 1, 0, 0, 0}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = motor.Apply(target)
	}
}

func BenchmarkMotorInterpolate(b *testing.B) {
	motor := MotorFromAxisAngle(0, 0, 1, math.Pi/2)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = motor.Interpolate(0.5)
	}
}
