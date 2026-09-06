package correlation_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestDependenceNext(t *testing.T) {
	tests.CheckDependence(t, correlation.NewDependence(algo.NewHayashiYoshida()))
}
