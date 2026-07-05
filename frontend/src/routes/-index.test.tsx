import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import { appStore, DEFAULT_KERNELS } from "#/collections/app";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import { Circular } from "#/collections/circular";
import { RouteComponent } from "./-index-component";

describe("index route", () => {
	afterEach(() => {
		appStore.setState((state) => ({
			...state,
			kernels: DEFAULT_KERNELS,
		}));
		measurementsStore.setState(() => ({
			measurements: {
				causal: Circular(50),
				correlation: Circular(50),
				cvd: Circular(50),
				depthflow: Circular(50),
				exhaustion: Circular(50),
				fluid: Circular(50),
				hawkes: Circular(50),
				leadlag: Circular(50),
				liquidity: Circular(50),
				manifold: Circular(50),
				pumpdump: Circular(50),
				regime: Circular(50),
				resonance: Circular(50),
				sentiment: Circular(50),
				toxicity: Circular(50),
			},
			symbols: {},
		}));
		manifoldStore.setState(() => ({
			frame: null,
			frames: [],
		}));
		resonanceStore.actions.reset();
		positionsStore.actions.reset();
		executionsStore.actions.reset();
		diagnosticsStore.actions.reset();
		decisionStore.actions.reset();
		terminalStore.setState((state) => ({
			...state,
			inspectorSource: null,
			selectedSource: "fluid",
			focusSymbol: "stream",
		}));
	});

	it("renders a visual lane while waiting for measurement frames", () => {
		appStore.setState((state) => ({
			...state,
			kernels: ["causal"],
		}));

		const html = renderToString(<RouteComponent />);

		expect(html).toContain("Causal ladder");
		expect(html).toContain("Standby");
		expect(html).toContain("waiting for decision frames");
	});

	it("renders a sparkline from the selected kernel's measurement history", () => {
		appStore.setState((state) => ({
			...state,
			kernels: ["causal"],
		}));
		measurementsStore.actions.updateFrame({
			source: "causal",
			symbol: "M/USD",
			category: "alpha",
			confidence: 0.8,
			status: "measured",
			surprise: 2.3,
			history: [
				{ confidence: 0.2 },
				{ confidence: 0.8 },
			],
		});

		const html = renderToString(<RouteComponent />);

		expect(html).toContain("Causal ladder");
		expect(html).toContain("0.0,23.8 150.0,8.2");
		expect(html).toContain("2.30");
		expect(html).toContain("x thr");
	});

	it("does not show confidence-bearing measurements as no status beside decisions", () => {
		appStore.setState((state) => ({
			...state,
			kernels: ["fluid"],
		}));
		measurementsStore.actions.updateFrame({
			source: "fluid",
			symbol: "SPX/USD",
			confidence: 0.97,
			strength: 0.7,
			surprise: 0,
		});
		decisionStore.actions.updateFrame({
			id: "decision-1",
			symbol: "SPX/USD",
			source: "trader",
			score: 0.458,
			verdict: "allow",
			reason: "admitted",
		});

		const html = renderToString(<RouteComponent />);

		expect(html).toContain("Healthy");
		expect(html).toContain("SPX/USD");
		expect(html).toContain("admitted");
		expect(html).not.toContain("No status");
	});
});
