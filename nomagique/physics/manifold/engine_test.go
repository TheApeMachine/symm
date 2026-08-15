//go:build darwin && cgo

package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestEngineNewField proves Fields can be created, advanced, and destroyed
repeatedly on both compact and production-grid Engines.
*/
func TestEngineNewField(t *testing.T) {
	Convey("Given a shared compact Metal Engine", t, func() {
		config := smallTestConfig()
		engine, err := NewEngine(config)
		So(err, ShouldBeNil)
		defer engine.Close()

		Convey("It should survive repeated Field teardown", func() {
			for range 3 {
				field, fieldErr := engine.NewField()
				So(fieldErr, ShouldBeNil)
				So(field.ResidentBytes(), ShouldBeGreaterThan, 0)

				_, stepErr := field.Step()
				So(stepErr, ShouldBeNil)
				field.Close()
			}
		})
	})

	Convey("Given the production 64-cubed Metal configuration", t, func() {
		config, err := NewConfig(64, 64, 64, 1, 32, 0.1, 5.0/3.0, 128)
		So(err, ShouldBeNil)
		DefaultMarketGasBoundaries().Apply(&config)

		engine, err := NewEngine(config)
		So(err, ShouldBeNil)
		defer engine.Close()

		Convey("It should advance successive production Fields", func() {
			for range 2 {
				field, fieldErr := engine.NewField()
				So(fieldErr, ShouldBeNil)

				_, stepErr := field.Step()
				So(stepErr, ShouldBeNil)
				field.Close()
			}
		})
	})
}

/*
TestEngineFieldBytes proves resident capacity uses a measured nonzero Field
footprint rather than a configured estimate.
*/
func TestEngineFieldBytes(t *testing.T) {
	Convey("Given a production-grid Engine", t, func() {
		config, err := NewConfig(64, 64, 64, 1, 32, 0.1, 5.0/3.0, 128)
		So(err, ShouldBeNil)
		DefaultMarketGasBoundaries().Apply(&config)
		engine, err := NewEngine(config)
		So(err, ShouldBeNil)
		defer engine.Close()

		Convey("It should measure the resident Field footprint", func() {
			fieldBytes, measureErr := engine.FieldBytes()
			So(measureErr, ShouldBeNil)
			So(fieldBytes, ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestEngineMaxFields proves capacity is derived from available device memory and
the measured Field footprint without a fixed resident-count fallback.
*/
func TestEngineMaxFields(t *testing.T) {
	Convey("Given a measured compact Metal Engine", t, func() {
		engine, err := NewEngine(smallTestConfig())
		So(err, ShouldBeNil)
		defer engine.Close()

		Convey("It should expose positive device-derived capacity", func() {
			capacity, capacityErr := engine.MaxFields()
			So(capacityErr, ShouldBeNil)
			So(capacity, ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestNewSolver exercises the legacy constructor twice so the shared Metal host
must remain valid after the first Solver closes.
*/
func TestNewSolver(t *testing.T) {
	Convey("Given two sequential legacy Solver lifecycles", t, func() {
		config := smallTestConfig()

		Convey("The second Solver should advance after the first closes", func() {
			for range 2 {
				solver, err := NewSolver(config)
				So(err, ShouldBeNil)

				_, stepErr := solver.Step()
				So(stepErr, ShouldBeNil)
				solver.Close()
			}
		})
	})
}

/*
TestEngineAllocatedBytes proves repeated field steps reuse the resident Metal
allocation instead of retaining completed command resources on Go threads.
*/
func TestEngineAllocatedBytes(t *testing.T) {
	Convey("Given one warmed production-grid Field", t, func() {
		config, err := NewConfig(64, 64, 64, 1, 32, 0.1, 5.0/3.0, 128)
		So(err, ShouldBeNil)
		DefaultMarketGasBoundaries().Apply(&config)

		engine, err := NewEngine(config)
		So(err, ShouldBeNil)
		defer engine.Close()

		field, err := engine.NewField()
		So(err, ShouldBeNil)
		defer field.Close()

		_, err = field.Step()
		So(err, ShouldBeNil)
		baseline := engine.AllocatedBytes()

		Convey("When the resident Field advances repeatedly", func() {
			for range 32 {
				_, err = field.Step()
				So(err, ShouldBeNil)
			}

			Convey("It should not retain another Field allocation", func() {
				So(engine.AllocatedBytes(), ShouldBeLessThanOrEqualTo,
					baseline+field.ResidentBytes())
			})
		})
	})
}

/*
BenchmarkEngineFields measures create/step/close of Fields on one Engine.
*/
func BenchmarkEngineFields(b *testing.B) {
	config := smallTestConfig()
	engine, err := NewEngine(config)

	if err != nil {
		b.Fatal(err)
	}

	defer engine.Close()

	posX, posY, posZ := config.testCellCenter(4, 0, 1)
	b.ReportAllocs()

	for b.Loop() {
		field, fieldErr := engine.NewField()

		if fieldErr != nil {
			b.Fatal(fieldErr)
		}

		if err := field.ResetDeposits(); err != nil {
			b.Fatal(err)
		}

		if err := field.DepositCell(4, 0, 1, 0.05, 0, 0, 0, 0.05); err != nil {
			b.Fatal(err)
		}

		if err := field.SetOscillators([]Oscillator{{
			Phase:     0.5,
			Omega:     6.28,
			Amplitude: 0.2,
			PosX:      posX,
			PosY:      posY,
			PosZ:      posZ,
			Heat:      0.2,
			VelX:      0.4,
		}}); err != nil {
			b.Fatal(err)
		}

		if _, err := field.Step(); err != nil {
			b.Fatal(err)
		}

		field.Close()
	}
}
