import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import { appStore } from "#/collections/app";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { Circular } from "#/collections/circular";
import { RouteComponent } from "./index";

describe("index route", () => {
	afterEach(() => {
		appStore.setState((state) => ({
			...state,
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
	});

	it("renders a visual lane while waiting for measurement artifacts", () => {
		appStore.setState((state) => ({
			...state,
			kernels: ["causal"],
		}));

		const html = renderToString(<RouteComponent />);

		expect(html).toContain("causal confidence");
		expect(html).toContain("waiting");
	});

	it("renders a sparkline from the selected kernel's measurement history", () => {
		appStore.setState((state) => ({
			...state,
			kernels: ["causal"],
		}));
		measurementsStore.actions.updateFrame({
			uuid: "measurement-1",
			role: "measurement",
			origin: "causal",
			scope: "M/USD",
			output: { category: "alpha", confidence: 0.2 },
		});
		measurementsStore.actions.updateFrame({
			uuid: "measurement-2",
			role: "measurement",
			origin: "causal",
			scope: "M/USD",
			output: { category: "alpha", confidence: 0.8 },
		});

		const html = renderToString(<RouteComponent />);

		expect(html).toContain("causal confidence");
		expect(html).toContain("0.000,0.800");
		expect(html).toContain("1.000,0.200");
	});
});
