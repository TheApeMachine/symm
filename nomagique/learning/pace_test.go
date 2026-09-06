package learning_test

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestPaceNext(t *testing.T) {
	tests.CheckPace(t, learning.NewPace(store.NewConstant(tests.Record(map[string]any{"rest": 0.03, "lower": 0.005, "upper": 0.15, "gain": 0.1, "band": 0.2, "window": 8.0}))))
}
