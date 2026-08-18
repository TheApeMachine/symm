import { createStore } from "@tanstack/react-store";

export const DEFAULT_KERNELS = [
	"correlation",
	"cvd",
	"depthflow",
	"exhaustion",
	"hawkes",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
	"toxicity",
];

export const DEFAULT_FOCUS_SYMBOL = "BTC/USD";

export type BacktestCapture = {
	id: number;
	startedAt: string;
	endedAt?: string;
	frames: number;
};

export type HindsightSignal = {
	at: string | null;
	graphScore: number;
	thesisScore: number;
	opportunity: boolean;
	opportunityType?: string;
	alternatives: Record<string, number> | null;
};

export type HindsightLeg = {
	symbol: string;
	buyAt: string;
	buyPrice: number;
	sellAt: string;
	sellPrice: number;
	profitPct: number;
};

export type HindsightOpportunity = {
	leg: HindsightLeg;
	signal: HindsightSignal;
	captured: boolean;
	missed: boolean;
};

export type HindsightSymbol = {
	symbol: string;
	upboundPct: number;
	realizedPct: number;
	missedPct: number;
	legs: number;
	missedLegs: number;
	opportunities: HindsightOpportunity[];
};

export type HindsightReport = {
	captureId: number;
	status?: string;
	symbols: HindsightSymbol[];
	missedPct: number;
	upboundPct: number;
	missedLegs: number;
	totalLegs: number;
};

export type BacktestState = {
	captureId: number | null;
	playing: boolean;
	position: string | null;
	startedAt: string | null;
	endedAt: string | null;
	rebooting: boolean;
	captures: BacktestCapture[];
	hindsight: HindsightReport | null;
};

export const appStore = createStore(
	{
		online: false,
		error: null as Record<string, unknown> | null,
		focusSymbol: DEFAULT_FOCUS_SYMBOL,
		query: "",
		kernels: DEFAULT_KERNELS,
		observedSources: new Set<string>(),
		/*
			The symbols the engine has actually said something about this run. It is
			not a list the backend publishes — no instruments key reaches the wire —
			so it accumulates from the frames that name a symbol. The command palette
			searches it, which is the only place a symbol the user has not already
			focused can be reached.
		*/
		symbols: [] as string[],
		startedAtMs: null as number | null,
		backtest: {
			captureId: null,
			playing: false,
			position: null,
			startedAt: null,
			endedAt: null,
			rebooting: false,
			captures: [],
			hindsight: null,
		} as BacktestState,
	},
	({ setState }) => ({
		updateBacktest: (frame: Partial<BacktestState>) =>
			setState((prev) => ({
				...prev,
				backtest: { ...prev.backtest, ...frame },
			})),
		setBacktestCaptures: (captures: BacktestCapture[]) =>
			setState((prev) => ({
				...prev,
				backtest: { ...prev.backtest, captures },
			})),
		updateOnline: (online: boolean) =>
			setState((prev) => ({
				...prev,
				online,
				startedAtMs: online ? (prev.startedAtMs ?? Date.now()) : null,
			})),
		updateError: (err: Record<string, unknown>) =>
			setState((prev) => ({
				...prev,
				error: err,
			})),
		clearError: () =>
			setState((prev) => ({
				...prev,
				error: null,
			})),
		updateFocusSymbol: (symbol: string) =>
			setState((prev) => ({
				...prev,
				focusSymbol: symbol,
			})),
		updateQuery: (query: string) =>
			setState((prev) => ({
				...prev,
				query: query,
			})),
		observeSymbols: (symbols: Iterable<string>) =>
			setState((prev) => {
				const merged = new Set(prev.symbols);
				let changed = false;

				for (const symbol of symbols) {
					if (symbol !== "" && !merged.has(symbol)) {
						merged.add(symbol);
						changed = true;
					}
				}

				return changed ? { ...prev, symbols: [...merged].sort() } : prev;
			}),
		observeSources: (sources: Set<string>) =>
			setState((prev) => {
				if (sources.size === 0) {
					return prev;
				}

				const merged = new Set(prev.observedSources);
				let changed = false;

				for (const source of sources) {
					if (!merged.has(source)) {
						merged.add(source);
						changed = true;
					}
				}

				if (!changed) {
					return prev;
				}

				const kernels = [...new Set([...DEFAULT_KERNELS, ...merged])];

				return {
					...prev,
					observedSources: merged,
					kernels,
				};
			}),
	}),
);
