package perspectives

import "math/rand"

func normalizeSearchDocument(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) Document {
	if document.Version <= 0 {
		document.Version = 1
	}

	if len(document.Playbooks) == 0 {
		document.Playbooks = append(
			document.Playbooks,
			randomPlaybookSpec(randomPlaybookName(random), profile, random),
		)
	}

	if len(document.Playbooks) > maxGeneratedPlaybooks {
		document.Playbooks = document.Playbooks[:maxGeneratedPlaybooks]
	}

	seen := make(map[string]struct{}, len(document.Playbooks))

	for index := range document.Playbooks {
		playbook := &document.Playbooks[index]
		playbook.Name = normalizePlaybookName(playbook.Name, seen, random)
		playbook.Regime = normalizeRegime(playbook.Regime, random)
		playbook.Policy = normalizePolicy(playbook.Policy, random)

		if playbook.Deny == nil {
			playbook.Deny = []BranchSpec{}
		}

		if len(playbook.Entry) == 0 {
			playbook.Entry = []BranchSpec{randomRoute(branchSectionEntry, profile, random)}
		}

		if len(playbook.Exit) == 0 {
			playbook.Exit = []BranchSpec{randomRoute(branchSectionExit, profile, random)}
		}

		playbook.Deny = pruneEntryShadowingDenies(playbook.Deny, playbook.Entry)
	}

	return document
}

func randomPlaybookName(random *rand.Rand) string {
	return searchPlaybookNames[random.Intn(len(searchPlaybookNames))]
}

func normalizePlaybookName(
	name string,
	seen map[string]struct{},
	random *rand.Rand,
) string {
	if _, err := parsePlaybookName(name); err == nil {
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}

			return name
		}
	}

	names := append([]string(nil), searchPlaybookNames...)
	random.Shuffle(len(names), func(leftIndex int, rightIndex int) {
		names[leftIndex], names[rightIndex] = names[rightIndex], names[leftIndex]
	})

	for _, candidate := range names {
		if _, exists := seen[candidate]; exists {
			continue
		}

		seen[candidate] = struct{}{}

		return candidate
	}

	fallback := names[len(seen)%len(names)]
	seen[fallback] = struct{}{}

	return fallback
}

func normalizeRegime(regime string, random *rand.Rand) string {
	if _, err := parseRegime(regime); err == nil {
		return regime
	}

	return searchRegimes[random.Intn(len(searchRegimes))]
}

func normalizePolicy(policy string, random *rand.Rand) string {
	if _, err := parsePolicy(policy); err == nil {
		return policy
	}

	return searchPolicies[random.Intn(len(searchPolicies))]
}
