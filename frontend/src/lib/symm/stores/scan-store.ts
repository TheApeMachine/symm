import { createStore } from "@tanstack/react-store";

export type ScanStoreState = {
	quotesReady: number;
	symbolsTotal?: number;
	fluidSampled: number;
};

export const scanStore = createStore<ScanStoreState>({
	quotesReady: 0,
	fluidSampled: 0,
});

export const applyQuoteProgress = (ready: number, total: number): void => {
	scanStore.setState((state) => ({
		...state,
		quotesReady: ready,
		symbolsTotal: total,
	}));
};

export const applyFluidSampled = (sampled: number): void => {
	scanStore.setState((state) => ({
		...state,
		fluidSampled: sampled,
	}));
};
