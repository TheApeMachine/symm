import { createStore } from "@tanstack/store";
import { useSyncExternalStore } from "react";

type DecisionSideState = {
	pendingFocusSymbol?: string;
	scopeSymbol?: string;
};

const decisionSideStore = createStore<DecisionSideState>({});

/*
setDecisionsScopeSymbol pins the active candidate for decision-rail paints.

The rails downstream are Components, and a Component selects its row with
data-filter — a real DOM attribute. So the pinned symbol has to reach React,
not just a module variable: subscribers re-render, the attribute changes, and
the next frame paints the newly selected candidate.
*/
export const setDecisionsScopeSymbol = (symbol: string | undefined): void => {
	if (decisionSideStore.state.scopeSymbol === symbol) {
		return;
	}

	decisionSideStore.setState((previous) => ({
		...previous,
		scopeSymbol: symbol,
	}));
};

/*
readDecisionsScopeSymbol returns the pinned candidate scope for DRAW paints.
*/
export const readDecisionsScopeSymbol = (): string | undefined =>
	decisionSideStore.state.scopeSymbol;

const subscribe = (listener: () => void): (() => void) => {
	const subscription = decisionSideStore.subscribe(listener);

	return () => subscription.unsubscribe();
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

/*
pendingFocusSymbol is a one-shot arrival flag, separate from the persistent
scope above. A dashboard row navigating in sets both: the scope so the side
rail already shows the right candidate, and this so the matching DecisionChain
expands and scrolls into view itself the first time its own painted symbol
matches — then clears it, so later live repaints of the same row don't keep
forcing it back open after the trader has collapsed or picked a different one.
*/
export const setDecisionsPendingFocus = (symbol: string): void => {
	decisionSideStore.setState((previous) => ({
		...previous,
		pendingFocusSymbol: symbol,
	}));
};

/*
consumeDecisionsPendingFocus returns and clears the pending focus symbol if it
matches, so only the first DecisionChain to observe the match claims it.
*/
export const consumeDecisionsPendingFocus = (symbol: string): boolean => {
	if (decisionSideStore.state.pendingFocusSymbol !== symbol) {
		return false;
	}

	decisionSideStore.setState((previous) => ({
		...previous,
		pendingFocusSymbol: undefined,
	}));

	return true;
};
