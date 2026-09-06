package data_test

import (
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestQualityNext(t *testing.T) { tests.CheckQuality(t, data.NewQuality(), data.NewAuthority()) }
