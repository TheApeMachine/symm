package joint_test

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/equation/joint"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestJointEstimatorNext(t *testing.T) {
	tests.CheckJoint(t, joint.NewEstimator(transport.NewIO(algo.NewWelford(), algo.NewWelford(), algo.NewWelford())))
}
