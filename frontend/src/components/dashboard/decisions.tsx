import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	setDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { DecisionList, type DecisionRow } from "#/components/ui/decision-list";
import { Decision } from "#/providers/telemetry/telemetry/decision";

const decObj = new Decision();

/*
selectDecisions folds the strategy frames into the latest decision standing
per symbol. It is exported because the graph-authored surfaces are handed
the same rows, and a second copy of this would be a second answer.
*/
export const selectDecisions = (stratState: any): DecisionRow[] => {
	const merged = new Map<string, DecisionRow>();
	const frames =
		typeof stratState?.toArray === "function"
			? stratState.toArray()
			: Array.isArray(stratState)
				? stratState
				: [];

	for (const frame of frames) {
		if (typeof frame?.decisionsLength !== "function") {
			if (Array.isArray(frame?.decisions)) {
				for (const dec of frame.decisions) {
					const symbol = dec.symbol ?? "";
					if (!symbol) continue;

					merged.set(symbol, {
						id: dec.id ?? `dec-${symbol}`,
						symbol,
						action: dec.action ?? "—",
						confidence:
							typeof dec.confidence === "number"
								? dec.confidence
								: typeof dec.confidence === "function"
									? dec.confidence()
									: 0,
						reason: dec.reason ?? "No rejection reason published",
					});
				}
			}
			continue;
		}

		for (let i = 0; i < frame.decisionsLength(); i++) {
			const dec = frame.decisions(i, decObj);
			if (!dec) {
				continue;
			}

			const symbol = dec.symbol() ?? "";
			if (!symbol) {
				continue;
			}

			merged.set(symbol, {
				id: dec.id() ?? `dec-${symbol}`,
				symbol,
				action: dec.action() ?? "—",
				confidence: dec.confidence(),
				reason: dec.reason() ?? "No rejection reason published",
			});
		}
	}

	return [...merged.values()];
};

/*
Decisions binds the strategy stream to the list that draws it.
*/
export const Decisions = () => {
	const decisions = useSelector(signals.strategy, selectDecisions);

	const inspectDecision = (symbol: string) => {
		setDecisionsScopeSymbol(symbol);
		setDecisionsPendingFocus(symbol);
		terminalStore.actions.openThesis(symbol);
	};

	return <DecisionList decisions={decisions} onInspect={inspectDecision} />;
};
