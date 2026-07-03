import { createStore } from "@tanstack/react-store";

export const appStore = createStore(
	{
		online: false,
		error: null as Record<string, unknown> | null,
		query: "",
		kernels: [
			"causal",
			"correlation",
			"cvd",
			"depthflow",
			"exhaustion",
			"fluid",
			"hawkes",
			"leadlag",
			"liquidity",
			"manifold",
			"pumpdump",
			"resonance",
			"sentiment",
			"toxicity",
		],
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
