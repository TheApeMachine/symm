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
	const anchor = layout?.anchor_symbol;

	if (typeof anchor === "string" && anchor.length > 0) {
		return anchor;
	}

	return "BTC/EUR";
};

export const useSymmStatus = () => undefined;

export const useSymmPositionSymbols = () => [] as string[];
