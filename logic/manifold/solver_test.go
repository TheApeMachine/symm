package manifold

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/signal/compute"
)

func TestSolverStep(t *testing.T) {
	Convey("Given a resident manifold domain with one physical carrier", t, func() {
		var domain *pfluid.Domain
		err := compute.WithMetalInit(func() error {
			created, createErr := pfluid.NewDomain(pfluid.DefaultConfig())

			if createErr != nil {
				return createErr
			}

			domain = created
			return nil
		})
		So(err, ShouldBeNil)
		Reset(func() { So(domain.Close(), ShouldBeNil) })
		_, err = domain.Append([]pfluid.Particle{{
			Position: pfluid.Vector{X: 0.5, Y: 0.5, Z: 0.5},
			Mass:     1,
			Heat:     0.1,
			Energy:   1,
			Phase:    0.1,
			Omega:    1,
		}}, []uint32{1})
		So(err, ShouldBeNil)
		binaryFrames := make(chan []byte, 1)
		solver := &Solver{domain: domain, binui: binaryFrames}
		at := time.Unix(1, 2).UTC()

		err = solver.Step("BTC/USD", at)

		Convey("It should publish the GPU display as an SMF1 binary packet", func() {
			So(err, ShouldBeNil)
			payload := <-binaryFrames
			So(string(payload[:4]), ShouldEqual, "SMF1")
			So(payload[4], ShouldEqual, binaryKindDisplay)
			So(binary.LittleEndian.Uint16(payload[5:7]), ShouldEqual, 64)
			So(binary.LittleEndian.Uint16(payload[7:9]), ShouldEqual, 64)
			So(string(payload[26:33]), ShouldEqual, "BTC/USD")
			So(len(payload[33:]), ShouldEqual, 64*64*4)
		})
	})

	Convey("Given a closed resident manifold domain", t, func() {
		var domain *pfluid.Domain
		err := compute.WithMetalInit(func() error {
			created, createErr := pfluid.NewDomain(pfluid.DefaultConfig())

			if createErr != nil {
				return createErr
			}

			domain = created
			return nil
		})
		So(err, ShouldBeNil)
		So(domain.Close(), ShouldBeNil)

		solver := &Solver{domain: domain}
		restoreLogging := errnie.SuppressLogging()
		defer restoreLogging()

		err = solver.Step("BTC/USD", time.Now().UTC())

		Convey("It should preserve the symbol, population, and fluid cause", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "BTC/USD")
			So(err.Error(), ShouldContainSubstring, "0 resident particles")
			So(err.Error(), ShouldContainSubstring, "fluid: domain is closed")
		})
	})
}

func TestEvict(t *testing.T) {
	Convey("Given a domain holding more particles than the field can integrate", t, func() {
		var domain *pfluid.Domain
		err := compute.WithMetalInit(func() error {
			created, createErr := pfluid.NewDomain(pfluid.DefaultConfig())

			if createErr != nil {
				return createErr
			}

			domain = created
			return nil
		})
		So(err, ShouldBeNil)
		Reset(func() { So(domain.Close(), ShouldBeNil) })

		residency := 8
		population := residency * 3

		particles := make([]pfluid.Particle, 0, population)
		contentIDs := make([]uint32, 0, population)

		for index := range population {
			particles = append(particles, pfluid.Particle{
				Position: pfluid.Vector{X: 0.5, Y: 0.5, Z: 0.5},
				Mass:     1,
				Heat:     0.1,
				Energy:   1,
				Phase:    0.1,
				Omega:    1,
			})
			contentIDs = append(contentIDs, uint32(index+1))
		}

		_, err = domain.Append(particles, contentIDs)
		So(err, ShouldBeNil)
		So(domain.ParticleCount(), ShouldEqual, population)

		solver := &Solver{domain: domain, residency: residency}
		solver.evict()

		Convey("It should hold residency at the configured bound", func() {
			/*
				Every symbol appends into one shared field and nothing leaves
				on its own, so without this bound the population grows until
				the pilot-wave transport goes non-finite and the manifold stops
				reading for the entire universe.
			*/
			So(domain.ParticleCount(), ShouldEqual, residency)
		})

		Convey("It should leave a domain already within the bound alone", func() {
			solver.evict()

			So(domain.ParticleCount(), ShouldEqual, residency)
		})
	})
}

func TestRejectBatch(t *testing.T) {
	Convey("Given a resident manifold domain with a destabilizing appended batch", t, func() {
		var domain *pfluid.Domain
		err := compute.WithMetalInit(func() error {
			created, createErr := pfluid.NewDomain(pfluid.DefaultConfig())

			if createErr != nil {
				return createErr
			}

			domain = created
			return nil
		})
		So(err, ShouldBeNil)
		Reset(func() { So(domain.Close(), ShouldBeNil) })

		particles := make([]pfluid.Particle, 0, 5)
		contentIDs := make([]uint32, 0, 5)

		for index := range 5 {
			particles = append(particles, pfluid.Particle{
				Position: pfluid.Vector{X: 0.5, Y: 0.5, Z: 0.5},
				Mass:     1,
				Heat:     0.1,
				Energy:   1,
				Phase:    0.1,
				Omega:    1,
			})
			contentIDs = append(contentIDs, uint32(index+1))
		}

		_, err = domain.Append(particles, contentIDs)
		So(err, ShouldBeNil)

		solver := &Solver{domain: domain, turnover: 5}
		solver.rejectBatch("BTC/USD", time.Now().UTC(), 3, 2, errors.New("failed test step"))

		Convey("It should retain the resident state that preceded that batch", func() {
			So(domain.ParticleCount(), ShouldEqual, 3)
			So(solver.turnover, ShouldEqual, 3)
		})
	})
}

func TestFilterBatch(t *testing.T) {
	Convey("Given an appended manifold batch with inadmissible particles", t, func() {
		solver := &Solver{config: pfluid.DefaultConfig()}
		particles := []pfluid.Particle{
			{
				Position: pfluid.Vector{X: 0.5, Y: 0.5, Z: 0.5},
				Mass:     1,
				Heat:     0.1,
				Energy:   1,
				Phase:    0.1,
				Omega:    0,
			},
			{
				Position: pfluid.Vector{X: float32(math.NaN()), Y: 0.5, Z: 0.5},
				Mass:     1,
				Heat:     0.1,
				Energy:   1,
				Phase:    0.1,
				Omega:    0,
			},
			{
				Position: pfluid.Vector{X: 0.5, Y: 0.5, Z: 0.5},
				Mass:     pfluid.MinimumPilotWaveMass,
				Heat:     0.1,
				Energy:   1,
				Phase:    0.1,
				Omega:    0,
			},
			{
				Position: pfluid.Vector{X: 0.5, Y: 0.5, Z: 0.5},
				Mass:     1,
				Heat:     -0.1,
				Energy:   1,
				Phase:    0.1,
				Omega:    0,
			},
		}
		contentIDs := []uint32{11, 12, 13, 14}

		filteredParticles, filteredContentIDs, dropped := solver.filterBatch(particles, contentIDs)

		Convey("It should retain only appendable particles", func() {
			So(dropped, ShouldEqual, 3)
			So(filteredParticles, ShouldHaveLength, 1)
			So(filteredContentIDs, ShouldResemble, []uint32{11})
			So(filteredParticles[0].Mass, ShouldEqual, float32(1))
			So(filteredParticles[0].Heat, ShouldEqual, float32(0.1))
			So(filteredParticles[0].Energy, ShouldEqual, float32(1))
		})
	})
}
