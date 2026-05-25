package geometry

import (
	"math"
	"testing"

	gc "github.com/smartystreets/goconvey/convey"
)

func TestRotorConstruction(t *testing.T) {
	gc.Convey("Given the Rotor constructor", t, func() {
		gc.Convey("When constructing a 90° rotation around e12", func() {
			rotor := Rotor(math.Pi/2, 1, 0, 0)

			gc.Convey("It should have cos(π/4) as scalar and sin(π/4) as e12", func() {
				gc.So(rotor[MvScalar], gc.ShouldAlmostEqual, math.Cos(math.Pi/4), 1e-12)
				gc.So(rotor[MvE12], gc.ShouldAlmostEqual, math.Sin(math.Pi/4), 1e-12)
				gc.So(rotor[MvE01], gc.ShouldEqual, 0)
				gc.So(rotor[MvE02], gc.ShouldEqual, 0)
				gc.So(rotor[MvE03], gc.ShouldEqual, 0)
				gc.So(rotor[MvE31], gc.ShouldEqual, 0)
				gc.So(rotor[MvE23], gc.ShouldEqual, 0)
				gc.So(rotor[MvE0123], gc.ShouldEqual, 0)
			})
		})

		gc.Convey("When constructing a 360° rotation", func() {
			rotor := Rotor(2*math.Pi, 0, 0, 1)

			gc.Convey("It should be approximately -1 (double cover of SO(3))", func() {
				gc.So(rotor[MvScalar], gc.ShouldAlmostEqual, -1.0, 1e-12)
				gc.So(math.Abs(rotor[MvE23]), gc.ShouldBeLessThan, 1e-12)
			})
		})

		gc.Convey("When constructing a zero-angle rotation", func() {
			rotor := Rotor(0, 1, 0, 0)

			gc.Convey("It should be the identity (scalar = 1)", func() {
				gc.So(rotor[MvScalar], gc.ShouldAlmostEqual, 1.0, 1e-12)
				gc.So(rotor[MvE12], gc.ShouldAlmostEqual, 0.0, 1e-12)
			})
		})
	})
}

func TestTranslatorConstruction(t *testing.T) {
	gc.Convey("Given the Translator constructor", t, func() {
		gc.Convey("When constructing a translation by (2, 4, 6)", func() {
			trans := Translator(2, 4, 6)

			gc.Convey("It should store half the displacement in the ideal bivectors", func() {
				gc.So(trans[MvScalar], gc.ShouldEqual, 1)
				gc.So(trans[MvE01], gc.ShouldEqual, 1.0)
				gc.So(trans[MvE02], gc.ShouldEqual, 2.0)
				gc.So(trans[MvE03], gc.ShouldEqual, 3.0)
				gc.So(trans[MvE12], gc.ShouldEqual, 0)
				gc.So(trans[MvE31], gc.ShouldEqual, 0)
				gc.So(trans[MvE23], gc.ShouldEqual, 0)
				gc.So(trans[MvE0123], gc.ShouldEqual, 0)
			})
		})
	})
}

func TestReverse(t *testing.T) {
	gc.Convey("Given a multivector with all components set", t, func() {
		mv := Multivector{1, 2, 3, 4, 5, 6, 7, 8}
		rev := mv.Reverse()

		gc.Convey("It should preserve grade-0 scalar unchanged", func() {
			gc.So(rev[MvScalar], gc.ShouldEqual, 1)
		})

		gc.Convey("It should negate all grade-2 bivector components", func() {
			gc.So(rev[MvE01], gc.ShouldEqual, -2)
			gc.So(rev[MvE02], gc.ShouldEqual, -3)
			gc.So(rev[MvE03], gc.ShouldEqual, -4)
			gc.So(rev[MvE12], gc.ShouldEqual, -5)
			gc.So(rev[MvE31], gc.ShouldEqual, -6)
			gc.So(rev[MvE23], gc.ShouldEqual, -7)
		})

		gc.Convey("It should preserve grade-4 pseudoscalar unchanged", func() {
			gc.So(rev[MvE0123], gc.ShouldEqual, 8)
		})
	})
}

func TestGeometricProduct(t *testing.T) {
	gc.Convey("Given PGA basis elements as multivectors", t, func() {
		identity := Multivector{1, 0, 0, 0, 0, 0, 0, 0}
		basisE01 := Multivector{0, 1, 0, 0, 0, 0, 0, 0}
		basisE12 := Multivector{0, 0, 0, 0, 1, 0, 0, 0}
		basisE31 := Multivector{0, 0, 0, 0, 0, 1, 0, 0}
		basisE23 := Multivector{0, 0, 0, 0, 0, 0, 1, 0}

		gc.Convey("When computing identity products", func() {
			gc.Convey("It should act as the multiplicative identity", func() {
				result := identity.GeometricProduct(basisE12)
				gc.So(result, gc.ShouldResemble, basisE12)

				result = basisE23.GeometricProduct(identity)
				gc.So(result, gc.ShouldResemble, basisE23)
			})
		})

		gc.Convey("When squaring Euclidean bivectors", func() {
			gc.Convey("It should give -1 for e12²", func() {
				result := basisE12.GeometricProduct(basisE12)
				gc.So(result[MvScalar], gc.ShouldEqual, -1.0)

				for idx := 1; idx < 8; idx++ {
					gc.So(result[idx], gc.ShouldEqual, 0.0)
				}
			})

			gc.Convey("It should give -1 for e31²", func() {
				result := basisE31.GeometricProduct(basisE31)
				gc.So(result[MvScalar], gc.ShouldEqual, -1.0)
			})

			gc.Convey("It should give -1 for e23²", func() {
				result := basisE23.GeometricProduct(basisE23)
				gc.So(result[MvScalar], gc.ShouldEqual, -1.0)
			})
		})

		gc.Convey("When squaring ideal bivectors", func() {
			gc.Convey("It should give 0 for e01² (degenerate metric)", func() {
				result := basisE01.GeometricProduct(basisE01)

				for idx := range 8 {
					gc.So(result[idx], gc.ShouldEqual, 0.0)
				}
			})
		})

		gc.Convey("When computing cyclic Euclidean products", func() {
			gc.Convey("It should give e12·e31 = +e23", func() {
				result := basisE12.GeometricProduct(basisE31)
				gc.So(result[MvE23], gc.ShouldEqual, 1.0)
				gc.So(result[MvScalar], gc.ShouldEqual, 0.0)
				gc.So(result[MvE12], gc.ShouldEqual, 0.0)
				gc.So(result[MvE31], gc.ShouldEqual, 0.0)
			})

			gc.Convey("It should give e31·e23 = +e12", func() {
				result := basisE31.GeometricProduct(basisE23)
				gc.So(result[MvE12], gc.ShouldEqual, 1.0)
				gc.So(result[MvScalar], gc.ShouldEqual, 0.0)
			})

			gc.Convey("It should give e23·e12 = +e31", func() {
				result := basisE23.GeometricProduct(basisE12)
				gc.So(result[MvE31], gc.ShouldEqual, 1.0)
				gc.So(result[MvScalar], gc.ShouldEqual, 0.0)
			})
		})

		gc.Convey("When computing anti-cyclic products", func() {
			gc.Convey("It should give e31·e12 = -e23", func() {
				result := basisE31.GeometricProduct(basisE12)
				gc.So(result[MvE23], gc.ShouldEqual, -1.0)
			})

			gc.Convey("It should give e12·e23 = -e31", func() {
				result := basisE12.GeometricProduct(basisE23)
				gc.So(result[MvE31], gc.ShouldEqual, -1.0)
			})

			gc.Convey("It should give e23·e31 = -e12", func() {
				result := basisE23.GeometricProduct(basisE31)
				gc.So(result[MvE12], gc.ShouldEqual, -1.0)
			})
		})

		gc.Convey("When computing unit rotor self-product R·R†", func() {
			rotor := Rotor(math.Pi/3, 0, 0, 1)
			rev := rotor.Reverse()
			product := rotor.GeometricProduct(rev)

			gc.Convey("It should produce the identity", func() {
				gc.So(product[MvScalar], gc.ShouldAlmostEqual, 1.0, 1e-12)

				for idx := 1; idx < 8; idx++ {
					gc.So(math.Abs(product[idx]), gc.ShouldBeLessThan, 1e-12)
				}
			})
		})

		gc.Convey("When checking associativity (A·B)·C = A·(B·C)", func() {
			mvA := Multivector{0.5, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7}
			mvB := Multivector{0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.1}
			mvC := Multivector{0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1, 0.8}

			leftAssoc := mvA.GeometricProduct(mvB).GeometricProduct(mvC)
			rightAssoc := mvA.GeometricProduct(mvB.GeometricProduct(mvC))

			gc.Convey("It should produce identical results", func() {
				for idx := range 8 {
					gc.So(leftAssoc[idx], gc.ShouldAlmostEqual, rightAssoc[idx], 1e-10)
				}
			})
		})
	})
}

func TestSandwich(t *testing.T) {
	gc.Convey("Given a 90° rotation rotor around the e23 axis", t, func() {
		rotor := Rotor(math.Pi/2, 0, 0, 1)
		target := Multivector{0, 0, 0, 0, 1, 0, 0, 0}

		gc.Convey("When sandwiching the e12 bivector", func() {
			result := rotor.Sandwich(target)

			gc.Convey("It should rotate e12 to e31 (xy-plane → zx-plane)", func() {
				gc.So(math.Abs(result[MvE12]), gc.ShouldBeLessThan, 1e-12)
				gc.So(result[MvE31], gc.ShouldAlmostEqual, 1.0, 1e-12)
				gc.So(math.Abs(result[MvE23]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(result[MvScalar]), gc.ShouldBeLessThan, 1e-12)
			})
		})
	})

	gc.Convey("Given a 180° rotation rotor around the e12 axis", t, func() {
		rotor := Rotor(math.Pi, 1, 0, 0)
		target := Multivector{0, 0, 0, 0, 0, 1, 0, 0}

		gc.Convey("When sandwiching the e31 bivector", func() {
			result := rotor.Sandwich(target)

			gc.Convey("It should negate e31 (half-turn in xy flips zx-plane orientation)", func() {
				gc.So(result[MvE31], gc.ShouldAlmostEqual, -1.0, 1e-12)
				gc.So(math.Abs(result[MvE12]), gc.ShouldBeLessThan, 1e-12)
				gc.So(math.Abs(result[MvE23]), gc.ShouldBeLessThan, 1e-12)
			})
		})
	})
}

func TestNormalize(t *testing.T) {
	gc.Convey("Given an unnormalized rotor-like multivector", t, func() {
		mv := Multivector{3, 0.1, 0.2, 0.3, 4, 0, 0, 0.5}
		norm := mv.Normalize()
		product := norm.GeometricProduct(norm.Reverse())

		gc.Convey("When normalized and multiplied by its reverse", func() {
			gc.Convey("It should produce scalar ≈ 1", func() {
				gc.So(product[MvScalar], gc.ShouldAlmostEqual, 1.0, 1e-10)
			})
		})
	})

	gc.Convey("Given an already-unit rotor", t, func() {
		rotor := Rotor(1.23, 0, 1, 0)
		norm := rotor.Normalize()

		gc.Convey("It should remain unchanged after normalization", func() {
			for idx := range 8 {
				gc.So(norm[idx], gc.ShouldAlmostEqual, rotor[idx], 1e-12)
			}
		})
	})
}

func TestCompose(t *testing.T) {
	gc.Convey("Given two 90° rotors around e12", t, func() {
		rot90 := Rotor(math.Pi/2, 1, 0, 0)
		composed := rot90.Compose(rot90)

		gc.Convey("When composed", func() {
			direct180 := Rotor(math.Pi, 1, 0, 0)

			gc.Convey("It should equal a single 180° rotation", func() {
				for idx := range 8 {
					gc.So(composed[idx], gc.ShouldAlmostEqual, direct180[idx], 1e-12)
				}
			})
		})
	})
}

func BenchmarkGeometricProduct(b *testing.B) {
	mvA := Rotor(0.7, 0.577, 0.577, 0.577)
	mvB := Rotor(1.2, 0, 0.707, 0.707)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = mvA.GeometricProduct(mvB)
	}
}

func BenchmarkSandwich(b *testing.B) {
	rotor := Rotor(0.7, 0.577, 0.577, 0.577)
	target := Multivector{0, 0, 0, 0, 1, 0, 0, 0}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = rotor.Sandwich(target)
	}
}

func BenchmarkNormalize(b *testing.B) {
	mv := Multivector{3.0, 0.1, 0.2, 0.3, 4.0, 5.0, 6.0, 0.5}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = mv.Normalize()
	}
}
