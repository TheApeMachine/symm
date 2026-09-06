package adaptive_test

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestWindowNext(t *testing.T) { tests.CheckWindow(t, adaptive.NewWindow()) }
