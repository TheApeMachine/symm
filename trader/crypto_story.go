package trader

import (
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

func groupMeasurementsByScope(
	measurements []*datura.Artifact,
) map[string][]*datura.Artifact {
	grouped := make(map[string][]*datura.Artifact)

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		scope, scopeErr := measurement.Scope()

		if scopeErr != nil || scope == "" {
			continue
		}

		grouped[scope] = append(grouped[scope], measurement)
	}

	return grouped
}

func collectScopes(
	measurements []*datura.Artifact,
	pairs *sync.Map,
) []string {
	seen := make(map[string]struct{})
	scopes := make([]string, 0)

	for scope := range groupMeasurementsByScope(measurements) {
		if _, ok := seen[scope]; ok {
			continue
		}

		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}

	lister := &scopeKeyLister{pairs: pairs}

	for _, symbol := range lister.keys() {
		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		scopes = append(scopes, symbol)
	}

	return scopes
}

type scopeKeyLister struct {
	pairs *sync.Map
}

func (keys *scopeKeyLister) keys() []string {
	if keys == nil || keys.pairs == nil {
		return nil
	}

	symbols := make([]string, 0)

	keys.pairs.Range(func(key, _ any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		symbols = append(symbols, symbol)

		return true
	})

	return symbols
}

func (crypto *Crypto) publishPlaybookWalk(
	scope string,
	measurements []*datura.Artifact,
) {
	if crypto == nil || crypto.story == nil || crypto.uiBroadcast == nil || scope == "" {
		return
	}

	playbookTree := crypto.story.PlaybookTree()

	if playbookTree == nil {
		return
	}

	walk := logic.WalkTree(scope, measurements, nil, playbookTree.Branches)

	crypto.storyTicks.Add(1)
	crypto.playbookEvaluations.Add(1)

	payload := map[string]any{
		"type":                 "decision_walk",
		"symbol":               scope,
		"steps":                walk.Steps,
		"active_path":          walk.ActivePath,
		"branches":             playbookTree.Branches,
		"story_ticks":          crypto.storyTicks.Load(),
		"playbook_evaluations": crypto.playbookEvaluations.Load(),
	}

	wire, err := sonic.Marshal(payload)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "trader: marshal decision walk", err))

		return
	}

	artifact := datura.Acquire("trader", datura.APPJSON)
	artifact.WithRole("story")
	artifact.WithScope(scope)
	artifact.WithDestination("ui")
	artifact.WithPayload(wire)

	errnie.Error(crypto.uiBroadcast.Send(artifact))
}

func (crypto *Crypto) insertMeasurement(measurement *datura.Artifact) {
	if crypto == nil || measurement == nil {
		return
	}

	stamped := crypto.tree.WithCognition(measurement)
	crypto.tree, _ = crypto.tree.InsertArtifact(measurement.Prefix(), stamped)

	errnie.Error(crypto.uiBroadcast.Send(stamped.WithDestination("ui")))
}
