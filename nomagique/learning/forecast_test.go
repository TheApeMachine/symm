package learning_test

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestForecastNext(t *testing.T) { tests.CheckCalibration(t, learning.NewForecast(), "forecast") }
