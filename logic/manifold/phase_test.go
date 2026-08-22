package manifold

import (
	"math"
	"math/cmplx"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRealizedDirection(t *testing.T) {
	Convey("Given forward returns and observable book scales", t, func() {
		Convey("A return within the scale dead zone should be flat", func() {
			So(realizedDirection(0.001, 0.005), ShouldEqual, "flat")
			So(realizedDirection(-0.001, 0.005), ShouldEqual, "flat")
			So(realizedDirection(0.0, 0.005), ShouldEqual, "flat")
		})

		Convey("A non-positive scale should classify as flat", func() {
			So(realizedDirection(0.01, 0.0), ShouldEqual, "flat")
			So(realizedDirection(0.01, -0.005), ShouldEqual, "flat")
		})

		Convey("A positive return exceeding scale should be up", func() {
			So(realizedDirection(0.01, 0.005), ShouldEqual, "up")
		})

		Convey("A negative return exceeding scale should be down", func() {
			So(realizedDirection(-0.01, 0.005), ShouldEqual, "down")
		})
	})
}

func TestOmegaBin(t *testing.T) {
	Convey("Given a frequency lattice span with 256 bins", t, func() {
		omegaMin := -1.0
		omegaMax := 1.0
		span := omegaMax - omegaMin
		bins := 256

		Convey("The lowest frequency should map to bin 0", func() {
			So(omegaBin(omegaMin, omegaMin, span, bins), ShouldEqual, 0)
		})

		Convey("The highest frequency should map to bin 255", func() {
			So(omegaBin(omegaMax, omegaMin, span, bins), ShouldEqual, 255)
		})

		Convey("The centre frequency should map to bin 128", func() {
			So(omegaBin(0.0, omegaMin, span, bins), ShouldEqual, 128)
		})

		Convey("Out-of-range frequencies should clamp cleanly to boundaries", func() {
			So(omegaBin(-10.0, omegaMin, span, bins), ShouldEqual, 0)
			So(omegaBin(10.0, omegaMin, span, bins), ShouldEqual, 255)
		})
	})
}

func TestOscillatorWave(t *testing.T) {
	Convey("Given resident oscillators with known phases and amplitudes", t, func() {
		oscillators := []Oscillator{
			{
				Omega:     0.5,
				Phase:     0.0,
				Amplitude: 2.0,
			},
			{
				Omega:     -0.5,
				Phase:     math.Pi / 2.0,
				Amplitude: 3.0,
			},
		}

		wave := oscillatorWave(oscillators)

		Convey("It should project mode real and imaginary components accurately", func() {
			So(wave, ShouldHaveLength, 2)
			So(float64(wave[0].Omega), ShouldAlmostEqual, 0.5, 1e-5)
			So(float64(wave[0].Real), ShouldAlmostEqual, 2.0, 1e-5)
			So(float64(wave[0].Imaginary), ShouldAlmostEqual, 0.0, 1e-5)

			So(float64(wave[1].Omega), ShouldAlmostEqual, -0.5, 1e-5)
			So(float64(wave[1].Real), ShouldAlmostEqual, 0.0, 1e-5)
			So(float64(wave[1].Imaginary), ShouldAlmostEqual, 3.0, 1e-5)
		})
	})
}

func TestProjectSourceDial(t *testing.T) {
	Convey("Given a population of oscillators", t, func() {
		omegaMin := -1.0
		omegaMax := 1.0

		Convey("Empty oscillators should yield an empty dial with no error", func() {
			dial, err := projectSourceDial(nil, omegaMin, omegaMax)
			So(err, ShouldBeNil)
			So(dial, ShouldBeNil)
		})

		Convey("Invalid omega span should return a validation error", func() {
			dial, err := projectSourceDial(
				[]Oscillator{{Omega: 0.0, Amplitude: 1.0}},
				1.0, -1.0,
			)
			So(err, ShouldNotBeNil)
			So(dial, ShouldBeNil)
		})

		Convey("Oscillators with positive amplitude should produce a finite PhaseDial", func() {
			oscillators := []Oscillator{
				{
					Omega:     0.0,
					Phase:     math.Pi / 4.0,
					Amplitude: 1.5,
				},
				{
					Omega:     0.5,
					Phase:     math.Pi / 2.0,
					Amplitude: 2.0,
				},
				{
					Omega:     -0.5,
					Phase:     math.Pi,
					Amplitude: 0.8,
				},
			}

			dial, err := projectSourceDial(oscillators, omegaMin, omegaMax)
			So(err, ShouldBeNil)
			So(dial, ShouldHaveLength, int(phaseLatticeWidth))

			Convey("Every dimension of the projected dial must be strictly finite", func() {
				for dimensionIndex := range dial {
					magnitude := cmplx.Abs(dial[dimensionIndex])
					So(math.IsNaN(magnitude), ShouldBeFalse)
					So(math.IsInf(magnitude, 0), ShouldBeFalse)
				}
			})

			Convey("Bins with resident oscillators should contain non-zero phasors", func() {
				centreBin := omegaBin(0.0, omegaMin, omegaMax-omegaMin, int(phaseLatticeWidth))
				expectedPhasor := complex(
					1.5*math.Cos(math.Pi/4.0),
					1.5*math.Sin(math.Pi/4.0),
				)
				So(real(dial[centreBin]), ShouldAlmostEqual, real(expectedPhasor), 1e-5)
				So(imag(dial[centreBin]), ShouldAlmostEqual, imag(expectedPhasor), 1e-5)
			})
		})

		Convey("Oscillators with zero or negative amplitude should be ignored", func() {
			oscillators := []Oscillator{
				{Omega: 0.0, Phase: 0.0, Amplitude: 0.0},
				{Omega: 0.5, Phase: 0.0, Amplitude: -1.0},
			}

			dial, err := projectSourceDial(oscillators, omegaMin, omegaMax)
			So(err, ShouldBeNil)
			So(dial, ShouldBeNil)
		})
	})
}

func BenchmarkProjectSourceDial(b *testing.B) {
	const orderCount = 68
	oscillators := make([]Oscillator, orderCount)

	for index := range orderCount {
		progress := float64(index) / float64(orderCount)
		oscillators[index] = Oscillator{
			Omega:     -1.0 + 2.0*progress,
			Phase:     2.0 * math.Pi * progress,
			Amplitude: 1.0 + progress,
		}
	}

	omegaMin := -1.0
	omegaMax := 1.0
	b.ReportAllocs()

	for b.Loop() {
		dial, err := projectSourceDial(oscillators, omegaMin, omegaMax)

		if err != nil {
			b.Fatal(err)
		}

		if len(dial) != int(phaseLatticeWidth) {
			b.Fatalf("got %d dimensions, want %d", len(dial), phaseLatticeWidth)
		}
	}
}
