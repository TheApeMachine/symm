package logic

type SubjectType string

const (
	SubjectNone       SubjectType = ""
	SubjectHolding    SubjectType = "holding"
	SubjectCategory   SubjectType = "category"
	SubjectConfidence SubjectType = "confidence"
	SubjectStrength   SubjectType = "strength"
	SubjectSurprise   SubjectType = "surprise"
	SubjectElapsed    SubjectType = "elapsed"
	SubjectEigenmode  SubjectType = "eigenmode"
)
