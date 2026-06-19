package logic

import (
	"github.com/theapemachine/datura"
)

type Signal interface {
	Measure(*datura.Artifact) *datura.Artifact
}
