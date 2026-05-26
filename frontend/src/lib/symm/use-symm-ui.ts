export {
	useSymmConnected,
	useSymmEnginePulse,
	useSymmTradePanelRows,
	useSymmWallet,
} from "#/lib/symm/use-dashboard-data";

export const useSymmDecisionTrace = () => undefined;

export const useSymmEntryLine = () => undefined;

export const useSymmEvaluations = () => [] as const;

export const useSymmScanProgress = () => ({
	quotesReady: 0,
	symbolsTotal: 0,
	fluidSampled: 0,
});

export const useMarketWatchSymbol = () => "BTC/EUR";

export const useSymmStatus = () => undefined;

export const useSymmPositionSymbols = () => [] as string[];
