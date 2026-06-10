const quoteCurrency = () =>
	(import.meta.env.VITE_ANCHOR_SYMBOL?.trim().split("/")[1] ||
		import.meta.env.VITE_QUOTE_CURRENCY?.trim() ||
		"USD") as string;

export const useSymmEntryLine = () => ({
	line: 0,
	median: 0,
	mad: 0,
});

export const useMarketWatchSymbol = () =>
	import.meta.env.VITE_ANCHOR_SYMBOL?.trim() || "BTC/USD";

export const useQuoteCurrency = () => quoteCurrency().toUpperCase();

export const useSymmStatus = () => undefined;

export const useSymmPositionSymbols = () => [] as string[];
