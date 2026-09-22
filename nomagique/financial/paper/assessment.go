package paper

import (
	"bytes"
	"context"
	"github.com/theapemachine/errnie"
)

/* AssessmentServer counts graph-supplied resolved truth once per example. */
type AssessmentServer struct {
	seen                                            map[string]string
	graded, correct, wrong, abstained, inapplicable uint64
}

func NewAssessment() *AssessmentServer { return &AssessmentServer{seen: make(map[string]string)} }
func (server *AssessmentServer) Write(ctx context.Context, call Assessment_write) error {
	args := call.Args()
	example, err := args.Example()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "assessment: example", err))
	}
	expected, err := args.Expected()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "assessment: expected class", err))
	}
	predicted, err := args.Predicted()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "assessment: predicted class", err))
	}
	if len(example) == 0 || len(expected) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "assessment: example and resolved truth required", nil))
	}
	key := string(example)
	if held, found := server.seen[key]; found {
		if held != string(expected) {
			return errnie.Error(errnie.Err(errnie.Validation, "assessment: conflicting truth", nil))
		}
		return nil
	}
	server.seen[key] = string(expected)
	if args.Holding() != args.RequiredHolding() {
		server.inapplicable++
		return nil
	}
	server.graded++
	if len(predicted) == 0 {
		server.abstained++
		return nil
	}
	if bytes.Equal(expected, predicted) {
		server.correct++
		return nil
	}
	server.wrong++
	return nil
}
func (server *AssessmentServer) Done(ctx context.Context, call Assessment_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "assessment: allocate", err))
	}
	result.SetGraded(server.graded)
	result.SetCorrect(server.correct)
	result.SetWrong(server.wrong)
	result.SetAbstained(server.abstained)
	result.SetInapplicable(server.inapplicable)
	return nil
}
