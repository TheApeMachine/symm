import { createStore } from "@tanstack/react-store";

export const DEFAULT_KERNELS = [
	"correlation",
	"cvd",
	"depthflow",
	"exhaustion",
	"fluid",
	"hawkes",
	"leadlag",
	"liquidity",
	"ohlc",
	"pumpdump",
	"sentiment",
	"toxicity",
];

export const appStore = createStore(
	{
		online: false,
		error: null as Record<string, unknown> | null,
		focusSymbol: "BTC/USD",
		query: "",
		kernels: DEFAULT_KERNELS,
		observedSources: new Set<string>(),
	},
	({ setState }) => ({
		updateOnline: (online: boolean) =>
			setState((prev) => ({
				...prev,
				online: online,
			})),
		updateError: (err: Record<string, unknown>) =>
			setState((prev) => ({
				...prev,
				error: err,
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
