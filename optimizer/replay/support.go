package replay

func deriveReentryTickCooldown(tickCount int, categoryCount int) int {
	if tickCount <= 1 {
		return 1
	}

	if categoryCount <= 0 {
		categoryCount = 1
	}

	cooldown := tickCount / (categoryCount * categoryCount)

	if cooldown < 1 {
		return 1
	}

	return cooldown
}
