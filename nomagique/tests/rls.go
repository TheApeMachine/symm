package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
	"math/rand"
	"testing"
)

// CheckRLS compares every prior prediction and posterior covariance against the
// original square-root implementation. Unlabeled queries must not train.
func CheckRLS(t *testing.T, node core.Primitive, dimension int, variance, lambda float64, sum core.Primitive) {
	t.Helper()
	reference, err := referenceNewRLS(referenceRLSConfig{Dimension: dimension, InitialVariance: variance, ForgettingFactor: lambda})
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewSource(818))
	for step := 0; step < 120; step++ {
		features := make([]float64, dimension)
		target := 0.5
		for i := range features {
			features[i] = random.NormFloat64()
			target += float64(i+1) * features[i]
		}
		target += random.NormFloat64() * 0.1
		input := map[string]any{"features": features}
		var expected referenceRLSOutput
		if step%7 == 0 {
			expected, err = reference.Predict(features)
		} else {
			input["target"] = target
			expected, err = reference.Measure(referenceRLSSample{Features: features, Target: target})
		}
		if err != nil {
			t.Fatal(err)
		}
		out := Drain(t, node, Values(Record(input)))
		Sound(t, node)
		if len(out) != 1 {
			t.Fatalf("step %d outputs %d", step, len(out))
		}
		fields := Fields(t, out[0])
		EqualNumber(t, Number(t, fields, "prediction"), expected.Value)
		EqualNumber(t, Number(t, fields, "scale"), expected.Scale)
		EqualNumber(t, Number(t, fields, "degrees_of_freedom"), expected.DegreesOfFreedom)
		if core.To[bool](fields["ready"]) != expected.Ready {
			t.Fatal("readiness differs")
		}
		if step%7 != 0 {
			EqualNumber(t, Number(t, fields, "innovation"), expected.Innovation)
		}
		if step%17 == 0 {
			rows := [][]float64{features, features, features}
			expectedSum, err := reference.PredictSum(rows)
			if err != nil {
				t.Fatal(err)
			}
			total := Drain(t, sum, Values(rows))
			Sound(t, sum)
			fields := Fields(t, total[0])
			EqualNumber(t, Number(t, fields, "prediction"), expectedSum.Value)
			EqualNumber(t, Number(t, fields, "scale"), expectedSum.Scale)
			EqualNumber(t, Number(t, fields, "degrees_of_freedom"), expectedSum.DegreesOfFreedom)
		}
		snapshot, err := reference.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		beta := core.To[[]float64](fields["beta"])
		root := core.To[[][]float64](fields["root"])
		for i := range beta {
			EqualNumber(t, beta[i], snapshot.Beta[i])
			for j := range beta {
				covariance := 0.0
				for k := range beta {
					covariance += root[i][k] * root[j][k]
				}
				expectedCov := snapshot.Covariance[i*len(beta)+j]
				if math.Abs(covariance-expectedCov) > 1e-9*(1+math.Abs(expectedCov)) {
					t.Fatalf("posterior covariance step %d: %g != %g", step, covariance, expectedCov)
				}
			}
		}
	}
}
