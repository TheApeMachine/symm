package learning_test

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestTrustWeightNext(t *testing.T) { tests.CheckCalibration(t, learning.NewTrustWeight(), "trust") }
