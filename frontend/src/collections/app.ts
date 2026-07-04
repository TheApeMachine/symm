import { createStore } from "@tanstack/react-store";

export const DEFAULT_KERNELS = [
	"fluid",
	"prediction",
	"hawkes",
	"resonance",
	"cognitive",
	"causal",
	"manifold",
	"regime",
	"correlation",
	"pumpdump",
	"toxicity",
	"exhaustion",
	"cvd",
	"depthflow",
	"liquidity",
	"sentiment",
	"leadlag",
];

export const appStore = createStore(
	{
		online: false,
		error: null as Record<string, unknown> | null,
		query: "",
		kernels: DEFAULT_KERNELS,
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
		updateQuery: (query: string) =>
			setState((prev) => ({
				...prev,
				query: query,
			})),
	}),
);
