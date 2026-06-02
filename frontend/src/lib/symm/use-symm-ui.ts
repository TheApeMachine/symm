import {
	useSymmConnected,
	useSymmDecisionTrace,
	useSymmEnginePulse,
	useSymmAuditRows,
	useSymmTradePanelRows,
	useSymmWallet,
} from "#/lib/symm/use-dashboard-data";
import { useDashboardLayout } from "#/lib/symm/use-dashboard-layout";

export {
	useSymmConnected,
	useSymmDecisionTrace,
	useSymmEnginePulse,
	useSymmAuditRows,
	useSymmTradePanelRows,
	useSymmWallet,
};

export const useSymmEntryLine = () => ({
	line: 0,
	median: 0,
	mad: 0,
});

export const useSymmEvaluations = () =>
	useSymmDecisionTrace()?.evaluations ?? [];

export const useSymmScanProgress = () => {
	const pulse = useSymmEnginePulse();

	return {
		quotesReady: pulse?.ticker_ready ?? 0,
		symbolsTotal: pulse?.symbols_total ?? 0,
		fluidSampled: pulse?.fluid_sampled ?? 0,
	};
};

export const useMarketWatchSymbol = () => {
	const layout = useDashboardLayout();

	if (layout.anchor_symbol !== undefined && layout.anchor_symbol.length > 0) {
		return layout.anchor_symbol;
	}

	return "BTC/EUR";
};

export const useSymmStatus = () => undefined;

export const useSymmPositionSymbols = () => [] as string[];
