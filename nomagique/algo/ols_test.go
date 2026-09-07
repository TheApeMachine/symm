package algo_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestOLSNext(t *testing.T) { tests.CheckOLS(t, algo.NewOLS(store.NewConstant(core.From(1e-15)))) }
