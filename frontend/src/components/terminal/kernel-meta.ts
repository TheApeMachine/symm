export type SignalHealthStatus =
	| "waiting"
	| "calibrating"
	| "fault"
	| "stale"
	| "flat"
	| "healthy";

export type KernelStatusMeta = {
	label: string;
	fg: string;
	bg: string;
	bd: string;
};

export const TERMINAL_KERNEL_ORDER = [
	"fluid",
	"prediction",
	"resonance",
	"hawkes",
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
] as const;

const kernelOrderIndex = (source: string): number => {
	const index = TERMINAL_KERNEL_ORDER.indexOf(
		source as (typeof TERMINAL_KERNEL_ORDER)[number],
	);

	return index === -1 ? TERMINAL_KERNEL_ORDER.length : index;
};

export const orderedKernelSources = (sources: string[]): string[] =>
	[...sources].sort((left, right) => {
		const byOrder = kernelOrderIndex(left) - kernelOrderIndex(right);

		return byOrder === 0 ? left.localeCompare(right) : byOrder;
	});

const KERNEL_COPY: Record<
	string,
	{ name: string; sub: string; blurb: string }
> = {
	fluid: {
		name: "Fluid dynamics",
		sub: "fluid · vol-rank × Δ",
		blurb:
			"Navier–Stokes pressure field over the market cross-section. Whale carriers bend the density surface; turbulence flags regime breaks before price confirms.",
	},
	hawkes: {
		name: "Hawkes process",
		sub: "hawkes · branching η",
		blurb:
			"Self-exciting point process over order-flow events. Branching ratio η near 1 means the book is reflexive and primed to cascade.",
	},
	// "prediction" is a presentation alias for the resonance measurement: the
	// backend has one autoencoder output, while the mockup names the detail row
	// after its predictive-coding role.
	prediction: {
		name: "Predictive coding",
		sub: "predict · 8-step horizon",
		blurb:
			"Hierarchical generative model. Each layer predicts the one below; the residual error norm is the tradeable surprise.",
	},
	resonance: {
		name: "Resonance",
		sub: "resonance · laminar/turbulent",
		blurb:
			"Latent-state x-ray of coupled oscillators. Laminar phase locks ride trends; turbulent decoherence precedes reversals.",
	},
	cognitive: {
		name: "Cognitive memory",
		sub: "cognitive · entropy gate",
		blurb:
			"Discrete DMT sequence sealed through an entropy gate, then a beam search lookahead picks the winning regime class.",
	},
	causal: {
		name: "Causal ladder",
		sub: "causal · assoc→interv→cf",
		blurb:
			"Pearl do-calculus. Climbs association → intervention → counterfactual to estimate the effect of acting, not merely observing.",
	},
	manifold: {
		name: "Manifold",
		sub: "manifold · whale carriers",
		blurb:
			"Density manifold projection of the liquidity field, with whale-carrier markers lifted off the base surface.",
	},
	regime: {
		name: "Regime radar",
		sub: "regime · 5-axis",
		blurb:
			"Cross-section mean of volatility, trend, bullish, bearish and choppiness — the coarse weather of the tape.",
	},
	correlation: {
		name: "Correlation",
		sub: "correlation · cross-section",
		blurb:
			"Cross-symbol correlation pressure from backend measurement artifacts.",
	},
	pumpdump: {
		name: "Pumpdump",
		sub: "pumpdump · ignition",
		blurb:
			"Pump impulse measurement from raw market artifacts projected into the terminal signal surface.",
	},
	leadlag: {
		name: "Lead-lag",
		sub: "leadlag · cross-lag",
		blurb:
			"Lead-lag coupling across the cross-section from backend measurement artifacts.",
	},
	liquidity: {
		name: "Liquidity",
		sub: "liquidity · depth",
		blurb:
			"Book depth and liquidity pressure from backend measurement artifacts.",
	},
	toxicity: {
		name: "Toxicity",
		sub: "toxicity · flow",
		blurb: "Order-flow toxicity measurement from backend artifacts.",
	},
	exhaustion: {
		name: "Exhaustion",
		sub: "exhaustion · fade",
		blurb: "Exhaustion measurement from backend artifacts.",
	},
	depthflow: {
		name: "Depthflow",
		sub: "depthflow · ladder",
		blurb: "Depth flow measurement from backend artifacts.",
	},
	cvd: {
		name: "CVD",
		sub: "cvd · pressure",
		blurb: "Cumulative volume delta pressure from backend artifacts.",
	},
	sentiment: {
		name: "Sentiment",
		sub: "sentiment · tape",
		blurb: "Sentiment measurement from backend artifacts.",
	},
};

export const kernelCopy = (source: string, category: string) =>
	KERNEL_COPY[source] ?? {
		name: source,
		sub: category || source,
		blurb:
			"Backend measurement emitted from raw artifacts and projected into the terminal signal surface.",
	};

export const kernelStatusMeta = (
	status: SignalHealthStatus,
): KernelStatusMeta => {
	const table: Record<SignalHealthStatus, KernelStatusMeta> = {
		healthy: {
			label: "Healthy",
			fg: "var(--up)",
			bg: "color-mix(in srgb, var(--up) 12%, transparent)",
			bd: "color-mix(in srgb, var(--up) 38%, transparent)",
		},
		stale: {
			label: "Stale",
			fg: "var(--down)",
			bg: "color-mix(in srgb, var(--down) 12%, transparent)",
			bd: "color-mix(in srgb, var(--down) 38%, transparent)",
		},
		fault: {
			label: "Fault",
			fg: "var(--down)",
			bg: "color-mix(in srgb, var(--down) 12%, transparent)",
			bd: "color-mix(in srgb, var(--down) 38%, transparent)",
		},
		waiting: {
			label: "Standby",
			fg: "var(--f3)",
			bg: "var(--line)",
			bd: "var(--line2)",
		},
		calibrating: {
			label: "Calib",
			fg: "var(--info)",
			bg: "color-mix(in srgb, var(--info) 12%, transparent)",
			bd: "color-mix(in srgb, var(--info) 38%, transparent)",
		},
		flat: {
			label: "Thin",
			fg: "var(--warn)",
			bg: "color-mix(in srgb, var(--warn) 12%, transparent)",
			bd: "color-mix(in srgb, var(--warn) 38%, transparent)",
		},
	};

	return table[status] ?? table.waiting;
};

export const kernelSparkPaths = (
	values: number[],
	surpriseRatio = 0,
): {
	spark: string;
	area: string;
	line: string;
	fill: string;
	firing: boolean;
} => {
	const history = values.length > 0 ? values : [0];
	const latest = history.at(-1);
	const firing = surpriseRatio >= 1 || (latest !== undefined && latest >= 0.58);
	const points = history.map((value, index) => {
		const x =
			history.length === 1
				? index === 0
					? "0.0"
					: "150.0"
				: ((index / Math.max(history.length - 1, 1)) * 150).toFixed(1);
		const y = (29 - value * 26).toFixed(1);

		return `${x},${y}`;
	});
	const spark =
		history.length === 1
			? `${points[0]} 150,${points[0].split(",")[1]}`
			: points.join(" ");
	const area = `${spark} 150,30 0,30`;

	return {
		spark,
		area,
		line: firing ? "var(--acc)" : "var(--info)",
		fill: firing
			? "color-mix(in srgb, var(--acc) 16%, transparent)"
			: "color-mix(in srgb, var(--info) 12%, transparent)",
		firing,
	};
};

export const formatUptime = (startedAtMs: number | null): string => {
	if (startedAtMs === null || !Number.isFinite(startedAtMs)) {
		return "0m 0s";
	}

	const totalSeconds = Math.max(
		0,
		Math.floor((Date.now() - startedAtMs) / 1000),
	);
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = totalSeconds % 60;

	return `${minutes}m ${seconds}s`;
};
