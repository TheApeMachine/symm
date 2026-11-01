package strategy

import "github.com/theapemachine/symm/system"

/*
Tier is one concurrent trading book. Virtual explores; paper validates;
real spends the live wallet once paper confidence is armed.
*/
type Tier string

const (
	TierVirtual Tier = "virtual"
	TierPaper   Tier = "paper"
	TierReal    Tier = "real"
)

func (tier Tier) String() string {
	return string(tier)
}

func (tier Tier) Virtual() bool {
	return tier == TierVirtual
}

/*
PaperEligible reports whether path resistance has cleared the paper floor.
*/
func PaperEligible(resistance float64) bool {
	return resistance <= PaperEligibleResistance()
}

/*
PaperEligibleResistance is the promotion floor (default 0.2).
*/
func PaperEligibleResistance() float64 {
	if system.Cfg != nil && system.Cfg.Learning != nil {
		if floor := system.Cfg.Learning.PaperEligibleResistance; floor > 0 {
			return floor
		}
	}

	return 0.2
}

/*
RealCeilingAllowed reports whether config permits live entries at all.
Paper mode keeps the real book structurally present but never arms.
*/
func RealCeilingAllowed() bool {
	if system.Cfg == nil || system.Cfg.Market == nil {
		return false
	}

	return system.Cfg.Market.Model == "real"
}
