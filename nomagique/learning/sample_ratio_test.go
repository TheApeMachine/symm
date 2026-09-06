package learning_test

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestSampleRatioNext(t *testing.T) { tests.CheckCalibration(t, learning.NewSampleRatio(), "ratio") }
