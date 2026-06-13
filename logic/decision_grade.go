package logic

/*
DecisionGrade separates diagnostic gauge output from execution-ready evidence.
*/
type DecisionGrade string

const (
	DecisionGradeNone       DecisionGrade = ""
	DecisionGradeDiagnostic DecisionGrade = "diagnostic"
	DecisionGradeExecutable DecisionGrade = "executable"
)

type SourceDecisionClass int

const (
	SourceClassFlow SourceDecisionClass = iota
	SourceClassTouch
	SourceClassComposite
)

/*
SourceDecisionClass groups sources by the evidence they require to become executable.
*/
func SourceDecisionClassFor(source SourceType) SourceDecisionClass {
	switch source {
	case SourceCVD, SourceHawkes, SourcePrediction:
		return SourceClassFlow
	case SourceDepthFlow, SourceLiquidity, SourceToxicity:
		return SourceClassTouch
	default:
		return SourceClassComposite
	}
}

/*
DecisionGradeFor assigns a grade from source class and touch readiness.
*/
func DecisionGradeFor(
	source SourceType,
	touchReady bool,
) DecisionGrade {
	if !touchReady {
		return DecisionGradeDiagnostic
	}

	switch SourceDecisionClassFor(source) {
	case SourceClassFlow, SourceClassTouch, SourceClassComposite:
		return DecisionGradeExecutable
	default:
		return DecisionGradeDiagnostic
	}
}
