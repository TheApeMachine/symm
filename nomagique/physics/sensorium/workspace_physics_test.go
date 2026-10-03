//go:build (darwin && cgo) || (linux && cuda && cgo)

package sensorium

import (
	"errors"
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func remapWorkspace(t testing.TB, grid, particles int) *workspace {
	t.Helper()
	fluid, err := newWorkspace(grid, grid, grid)

	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fluid.Close)
	fluid.allocateParticles(particles)
	fluid.particles = particles

	for particle := range particles {
		fluid.mass.Float32Slice()[particle] = 1
		fluid.heat.Float32Slice()[particle] = 2
		fluid.materialEnergy.Float32Slice()[particle] = 2

		for axis := range 3 {
			fluid.pos.Float32Slice()[3*particle+axis] = float32(math.Mod(float64(particle+1)*math.Sqrt(float64(axis+2)), 1))
		}
	}

	if err := fluid.depositDual(); err != nil {
		t.Fatal(err)
	}
	return fluid
}

func TestGatherDual(t *testing.T) {
	Convey("Exhausted remap iterations reject the candidate without timestep retries", t, func() {
		fluid, err := newWorkspace(64, 64, 64)
		So(err, ShouldBeNil)
		Reset(fluid.Close)
		fluid.loadState(capturedBootState(t))
		bound, err := fluid.stabilityLimit()
		So(err, ShouldBeNil)
		interval := float32(bound / 2)
		So(fluid.gasDual(interval), ShouldBeNil)
		velocity := append([]float32(nil), fluid.vel.Float32Slice()...)
		thermal := append([]float32(nil), fluid.heat.Float32Slice()...)
		fluid.physics.RemapIterations = 1
		err = fluid.gatherDual(interval)
		So(err, ShouldNotBeNil)
		var numerical *CoupledStepError
		So(errors.As(err, &numerical), ShouldBeTrue)
		So(numerical.Retry, ShouldBeFalse)
		So(fluid.vel.Float32Slice(), ShouldResemble, velocity)
		So(fluid.heat.Float32Slice(), ShouldResemble, thermal)
	})
	for _, grid := range []int{8, 64} {
		t.Run(fmt.Sprint(grid), func(t *testing.T) {
			Convey("Sparse material remaps without changing integrated energy", t, func() {
				fluid := remapWorkspace(t, grid, 16)
				So(fluid.gatherDual(0.0001), ShouldBeNil)
				So(fluid.materialTotal(), ShouldAlmostEqual, 32, 0.001)
				t.Logf("remap: %+v", fluid.health.Remap)
			})
		})
	}
}

func BenchmarkGatherDual(b *testing.B) {
	fluid := remapWorkspace(b, 64, 16)
	b.ResetTimer()

	for b.Loop() {
		if err := fluid.gatherDual(0.0001); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDepositDual(t *testing.T) {
	Convey("Material deposition preserves a sparse particle's mass and energy", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(fluid.Close)
		fluid.allocateParticles(1)
		fluid.particles = 1
		fluid.mass.Float32Slice()[0] = 1
		fluid.heat.Float32Slice()[0] = 2
		fluid.materialEnergy.Float32Slice()[0] = 2
		So(fluid.depositDual(), ShouldBeNil)
		var mass, energy float64
		vacuum := 0
		volume := fluid.domain.GridSpacing() * fluid.domain.GridSpacing() * fluid.domain.GridSpacing()

		for cell := range fluid.domain.CellCount() {
			state := fluid.hydro.Float32Slice()[6*cell : 6*cell+6]
			mass += float64(state[0]) * volume
			energy += float64(state[4]) * volume

			if state[0] == 0 {
				vacuum++
				So(state, ShouldResemble, []float32{0, 0, 0, 0, 0, 0})
			}
		}
		So(mass, ShouldEqual, 1)
		So(energy, ShouldEqual, 2)
		So(vacuum, ShouldEqual, fluid.domain.CellCount()-1)
	})
}

func TestCheckFlags(t *testing.T) {
	Convey("Only timestep-dependent numerical failures may retry", t, func() {
		fluid := remapWorkspace(t, 8, 1)
		for _, status := range []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9} {
			fluid.particleStatus.UInt32Slice()[0] = status
			err := checkFlags("regression", fluid.particleStatus, 1)
			So(err, ShouldNotBeNil)
			So(err.(*CoupledStepError).Retry, ShouldEqual, status == 3 || status == 4)
		}
	})
}

func TestExportDual(t *testing.T) {
	Convey("Primitive recovery preserves low densities and high bulk velocities", t, func() {
		fluid := remapWorkspace(t, 8, 1)
		for _, regime := range []struct{ density, speed float32 }{{1, 3}, {1e-10, 2000}} {
			fluid.hydro.Zero()
			state := fluid.hydro.Float32Slice()
			state[0] = regime.density
			state[1] = regime.density * regime.speed
			state[4] = regime.density * (.5*regime.speed*regime.speed + 1)
			state[5] = regime.density
			So(fluid.engine.ExportDual(fluid.hydro, fluid.rho, fluid.mom, fluid.energy, fluid.hydroStatus, fluid.hydroParams(1)), ShouldBeNil)
			So(fluid.rho.Float32Slice()[0], ShouldEqual, regime.density)
			So(fluid.mom.Float32Slice()[0]/fluid.rho.Float32Slice()[0], ShouldAlmostEqual, regime.speed, 1e-3)
		}
	})
}
