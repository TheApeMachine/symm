package system

import "github.com/spf13/viper"

type CVD struct {
	MinEvidenceCount int
}

func NewCVD() *CVD {
	viper.SetDefault("cvd.min_evidence_count", 2)

	return &CVD{
		MinEvidenceCount: viper.GetInt("cvd.min_evidence_count"),
	}
}
