package manifold

import (
	"encoding/binary"
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
