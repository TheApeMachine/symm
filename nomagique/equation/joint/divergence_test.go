package joint_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation/joint"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNewDivergence(t *testing.T) {
	tests.CheckJointDivergence(t, joint.NewDivergence(
		transport.NewIO(algo.NewWelford(), algo.NewWelford()),
		transport.NewIO(linear.NewLocalRegression(), linear.NewLocalRegression())))
	left := algo.NewWelford()
	tests.CheckJointRegression(t, joint.NewDivergence(
		transport.NewIO(left, algo.NewWelford()),
		transport.NewIO(linear.NewLocalRegression(), linear.NewLocalRegression())), left)
}

func BenchmarkNewDivergence(b *testing.B) {
	divergence := joint.NewDivergence(
		transport.NewIO(algo.NewWelford(), algo.NewWelford()),
		transport.NewIO(linear.NewLocalRegression(), linear.NewLocalRegression()),
	)
	var timestamp int64
	b.ReportAllocs()

	for iteration := 0; iteration < b.N; iteration++ {
		input := transport.NewIO(tests.Record(map[string]any{
			"values": []float64{1, -2}, "at": timestamp,
		}))

		if divergence.Next(input) == nil || divergence.Next(input) != nil {
			b.Fatal("expected one joint divergence record")
		}

		timestamp += 1e9
	}

	if err := divergence.Error(); err != nil {
		b.Fatal(err)
	}
}
