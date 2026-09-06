package probability_test

import (
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestDistributionSnapshot(t *testing.T) {
	tests.CheckDistribution(t, probability.NewDistribution())
}
