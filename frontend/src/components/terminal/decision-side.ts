import { useSyncExternalStore } from "react";

let decisionsScopeSymbol: string | undefined;

const listeners = new Set<() => void>();

/*
setDecisionsScopeSymbol pins the active candidate for decision-rail paints.

The rails downstream are Components, and a Component selects its row with
data-filter — a real DOM attribute. So the pinned symbol has to reach React,
not just a module variable: subscribers re-render, the attribute changes, and
the next frame paints the newly selected candidate.
*/
export const setDecisionsScopeSymbol = (symbol: string | undefined): void => {
	if (decisionsScopeSymbol === symbol) {
		return;
	}

	decisionsScopeSymbol = symbol;

	for (const listener of listeners) {
		listener();
	}
};

/*
readDecisionsScopeSymbol returns the pinned candidate scope for DRAW paints.
*/
export const readDecisionsScopeSymbol = (): string | undefined =>
	decisionsScopeSymbol;

const subscribe = (listener: () => void) => {
	listeners.add(listener);

	return () => {
		listeners.delete(listener);
	};
};

/*
useDecisionsScopeSymbol reads the pinned candidate in a component.
*/
export const useDecisionsScopeSymbol = (): string | undefined =>
	useSyncExternalStore(
		subscribe,
		readDecisionsScopeSymbol,
		readDecisionsScopeSymbol,
	);
