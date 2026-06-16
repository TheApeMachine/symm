package logic

type SubjectType string

const (
	SubjectNone       SubjectType = ""
	SubjectCategory   SubjectType = "category"
	SubjectRegime     SubjectType = "regime"
	SubjectPosition   SubjectType = "position"
	SubjectHolding    SubjectType = "holding"
	SubjectPrice      SubjectType = "price"
	SubjectVolume     SubjectType = "volume"
	SubjectSpread     SubjectType = "spread"
	SubjectElapsed    SubjectType = "elapsed"
	SubjectStrength   SubjectType = "strength"
	SubjectConfidence SubjectType = "confidence"
	SubjectSurprise   SubjectType = "surprise"
	SubjectEigenmode  SubjectType = "eigenmode"
	SubjectModeShare  SubjectType = "mode_share"
)
