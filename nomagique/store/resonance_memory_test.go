package store

import (
	. "github.com/smartystreets/goconvey/convey"
	"math"
	"testing"
)

func TestResonanceMemoryObserve(t *testing.T) {
	Convey("Measured temporal covariance separates coherent from rapidly changing inputs", t, func() {
		coherent := resonanceMemory{}
		varying := resonanceMemory{}
		coherentReach, varyingReach := 0, 0
		for observation := range 128 {
			coherentReach = coherent.observe([]float64{float64(observation), float64(observation) * 2})
			varyingReach = varying.observe([]float64{math.Sin(float64(observation * observation)), math.Cos(float64(observation*observation) / 3)})
		}
		So(coherentReach, ShouldEqual, 127)
		So(varyingReach, ShouldEqual, 1)
	})
}
