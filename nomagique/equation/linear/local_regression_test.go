package linear_test

import (
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestLocalRegressionNext(t *testing.T) {
	tests.CheckLocalRegression(t, linear.NewLocalRegression())
}
