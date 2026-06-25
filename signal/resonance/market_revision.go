package resonance

func (signal *Signal) markMarketChanged(symbol string) {
	if signal == nil || symbol == "" {
		return
	}

	revision := uint64(0)

	if raw, ok := signal.marketRevision.Load(symbol); ok {
		if stored, storedOK := raw.(uint64); storedOK {
			revision = stored
		}
	}

	signal.marketRevision.Store(symbol, revision+1)
}

func (signal *Signal) filterChangedScopes(scopes []string) []string {
	if signal == nil || len(scopes) == 0 {
		return nil
	}

	changed := make([]string, 0, len(scopes))

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		marketRaw, marketOK := signal.marketRevision.Load(scope)

		if !marketOK {
			changed = append(changed, scope)

			continue
		}

		marketRevision, revisionOK := marketRaw.(uint64)

		if !revisionOK {
			continue
		}

		settleRaw, settleOK := signal.lastSettleRevision.Load(scope)

		if settleOK {
			settleRevision, settledOK := settleRaw.(uint64)

			if settledOK && settleRevision == marketRevision {
				continue
			}
		}

		changed = append(changed, scope)
	}

	return changed
}

func (signal *Signal) rememberSettledScopes(scopes []string) {
	if signal == nil {
		return
	}

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		marketRaw, marketOK := signal.marketRevision.Load(scope)

		if !marketOK {
			continue
		}

		marketRevision, revisionOK := marketRaw.(uint64)

		if !revisionOK {
			continue
		}

		signal.lastSettleRevision.Store(scope, marketRevision)
	}
}
