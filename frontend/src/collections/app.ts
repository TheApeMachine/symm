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
	},
	({ setState }) => ({
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
