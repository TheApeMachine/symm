package perspectives

import "math/rand"

const searchActionSampleCount = 32

type documentActionKind uint8

const (
	documentActionAddPlaybook documentActionKind = iota
	documentActionRemovePlaybook
	documentActionSetRegime
	documentActionSetPolicy
	documentActionAddBranch
	documentActionReplaceBranch
	documentActionRemoveBranch
	documentActionGrowBranch
	documentActionSwapBranches
)

type documentAction struct {
	kind          documentActionKind
	playbookIndex int
	branchIndex   int
	section       branchSection
	playbook      PlaybookSpec
	branch        BranchSpec
	regime        string
	policy        string
}

func searchActions(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) []documentAction {
	document = normalizeSearchDocument(document, profile, random)
	actions := make([]documentAction, 0, searchActionSampleCount)

	for len(actions) < searchActionSampleCount {
		action, ok := randomDocumentAction(document, profile, random)

		if !ok {
			break
		}

		actions = append(actions, action)
	}

	return actions
}

func randomDocumentAction(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) (documentAction, bool) {
	if len(document.Playbooks) == 0 {
		return documentAction{kind: documentActionAddPlaybook}, true
	}

	switch random.Intn(9) {
	case 0:
		return randomAddPlaybookAction(document, profile, random)
	case 1:
		return randomRemovePlaybookAction(document, random)
	case 2:
		return randomSetRegimeAction(document, random), true
	case 3:
		return randomSetPolicyAction(document, random), true
	case 4:
		return randomAddBranchAction(document, profile, random), true
	case 5:
		return randomReplaceBranchAction(document, profile, random)
	case 6:
		return randomRemoveBranchAction(document, random)
	case 7:
		return randomGrowBranchAction(document, profile, random)
	default:
		return randomSwapBranchesAction(document, random)
	}
}

func randomAddPlaybookAction(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) (documentAction, bool) {
	if len(document.Playbooks) >= maxGeneratedPlaybooks {
		return randomAddBranchAction(document, profile, random), true
	}

	seen := make(map[string]struct{}, len(document.Playbooks))

	for _, playbook := range document.Playbooks {
		seen[playbook.Name] = struct{}{}
	}

	name := normalizePlaybookName("", seen, random)

	return documentAction{
		kind:     documentActionAddPlaybook,
		playbook: randomPlaybookSpec(name, profile, random),
	}, true
}

func randomRemovePlaybookAction(
	document Document,
	random *rand.Rand,
) (documentAction, bool) {
	if len(document.Playbooks) <= 1 {
		return randomSetRegimeAction(document, random), true
	}

	return documentAction{
		kind:          documentActionRemovePlaybook,
		playbookIndex: random.Intn(len(document.Playbooks)),
	}, true
}

func randomSetRegimeAction(document Document, random *rand.Rand) documentAction {
	return documentAction{
		kind:          documentActionSetRegime,
		playbookIndex: random.Intn(len(document.Playbooks)),
		regime:        searchRegimes[random.Intn(len(searchRegimes))],
	}
}

func randomSetPolicyAction(document Document, random *rand.Rand) documentAction {
	return documentAction{
		kind:          documentActionSetPolicy,
		playbookIndex: random.Intn(len(document.Playbooks)),
		policy:        searchPolicies[random.Intn(len(searchPolicies))],
	}
}

func randomAddBranchAction(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) documentAction {
	section := randomSection(random)

	return documentAction{
		kind:          documentActionAddBranch,
		playbookIndex: random.Intn(len(document.Playbooks)),
		section:       section,
		branch:        randomRoute(section, profile, random),
	}
}

func randomReplaceBranchAction(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) (documentAction, bool) {
	playbookIndex := random.Intn(len(document.Playbooks))
	section := randomSection(random)
	branches := branchSectionFor(&document.Playbooks[playbookIndex], section)

	if len(*branches) == 0 {
		return randomAddBranchAction(document, profile, random), true
	}

	return documentAction{
		kind:          documentActionReplaceBranch,
		playbookIndex: playbookIndex,
		branchIndex:   random.Intn(len(*branches)),
		section:       section,
		branch:        randomRoute(section, profile, random),
	}, true
}

func randomRemoveBranchAction(
	document Document,
	random *rand.Rand,
) (documentAction, bool) {
	playbookIndex := random.Intn(len(document.Playbooks))
	section := randomSection(random)
	branches := branchSectionFor(&document.Playbooks[playbookIndex], section)

	if !canRemoveBranch(section, len(*branches)) {
		return randomSetPolicyAction(document, random), true
	}

	return documentAction{
		kind:          documentActionRemoveBranch,
		playbookIndex: playbookIndex,
		branchIndex:   random.Intn(len(*branches)),
		section:       section,
	}, true
}

func randomGrowBranchAction(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) (documentAction, bool) {
	playbookIndex := random.Intn(len(document.Playbooks))
	section := randomSection(random)
	branches := branchSectionFor(&document.Playbooks[playbookIndex], section)

	if len(*branches) == 0 {
		return randomAddBranchAction(document, profile, random), true
	}

	return documentAction{
		kind:          documentActionGrowBranch,
		playbookIndex: playbookIndex,
		branchIndex:   random.Intn(len(*branches)),
		section:       section,
		branch:        randomBranchChain(section, profile, random),
	}, true
}

func randomSwapBranchesAction(
	document Document,
	random *rand.Rand,
) (documentAction, bool) {
	playbookIndex := random.Intn(len(document.Playbooks))
	section := randomSection(random)
	branches := branchSectionFor(&document.Playbooks[playbookIndex], section)

	if len(*branches) < 2 {
		return randomSetRegimeAction(document, random), true
	}

	return documentAction{
		kind:          documentActionSwapBranches,
		playbookIndex: playbookIndex,
		branchIndex:   random.Intn(len(*branches) - 1),
		section:       section,
	}, true
}

func randomSection(random *rand.Rand) branchSection {
	switch random.Intn(3) {
	case 0:
		return branchSectionEntry
	case 1:
		return branchSectionDeny
	default:
		return branchSectionExit
	}
}

func (action documentAction) Apply(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) Document {
	document = cloneDocument(document)

	switch action.kind {
	case documentActionAddPlaybook:
		document.Playbooks = append(document.Playbooks, clonePlaybook(action.playbook))
	case documentActionRemovePlaybook:
		document.Playbooks = removePlaybookAt(document.Playbooks, action.playbookIndex)
	case documentActionSetRegime:
		if playbook := playbookAt(document, action.playbookIndex); playbook != nil {
			playbook.Regime = action.regime
		}
	case documentActionSetPolicy:
		if playbook := playbookAt(document, action.playbookIndex); playbook != nil {
			playbook.Policy = action.policy
		}
	default:
		document = action.applyBranchAction(document)
	}

	return normalizeSearchDocument(document, profile, random)
}

func (action documentAction) applyBranchAction(document Document) Document {
	playbook := playbookAt(document, action.playbookIndex)

	if playbook == nil {
		return document
	}

	branches := branchSectionFor(playbook, action.section)

	switch action.kind {
	case documentActionAddBranch:
		*branches = append(*branches, cloneBranch(action.branch))
	case documentActionReplaceBranch:
		if action.branchIndex >= 0 && action.branchIndex < len(*branches) {
			(*branches)[action.branchIndex] = cloneBranch(action.branch)
		}
	case documentActionRemoveBranch:
		if canRemoveBranch(action.section, len(*branches)) {
			*branches = removeBranchAt(*branches, action.branchIndex)
		}
	case documentActionGrowBranch:
		if action.branchIndex >= 0 && action.branchIndex < len(*branches) {
			(*branches)[action.branchIndex].Branches = append(
				(*branches)[action.branchIndex].Branches,
				cloneBranch(action.branch),
			)
		}
	case documentActionSwapBranches:
		if action.branchIndex >= 0 && action.branchIndex+1 < len(*branches) {
			(*branches)[action.branchIndex], (*branches)[action.branchIndex+1] =
				(*branches)[action.branchIndex+1], (*branches)[action.branchIndex]
		}
	}

	return document
}

func playbookAt(document Document, index int) *PlaybookSpec {
	if index < 0 || index >= len(document.Playbooks) {
		return nil
	}

	return &document.Playbooks[index]
}

func branchSectionFor(
	playbook *PlaybookSpec,
	section branchSection,
) *[]BranchSpec {
	switch section {
	case branchSectionDeny:
		return &playbook.Deny
	case branchSectionExit:
		return &playbook.Exit
	default:
		return &playbook.Entry
	}
}

func removePlaybookAt(playbooks []PlaybookSpec, index int) []PlaybookSpec {
	if index < 0 || index >= len(playbooks) || len(playbooks) <= 1 {
		return playbooks
	}

	return append(playbooks[:index], playbooks[index+1:]...)
}

func removeBranchAt(branches []BranchSpec, index int) []BranchSpec {
	if index < 0 || index >= len(branches) {
		return branches
	}

	return append(branches[:index], branches[index+1:]...)
}

func canRemoveBranch(section branchSection, count int) bool {
	if section == branchSectionDeny {
		return count > 0
	}

	return count > 1
}

func clonePlaybook(playbook PlaybookSpec) PlaybookSpec {
	return PlaybookSpec{
		Name:   playbook.Name,
		Regime: playbook.Regime,
		Policy: playbook.Policy,
		Deny:   cloneBranchList(playbook.Deny),
		Entry:  cloneBranchList(playbook.Entry),
		Exit:   cloneBranchList(playbook.Exit),
	}
}
