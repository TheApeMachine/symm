import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DashboardSurface } from "#/components/terminal/dashboard";
import {
	kernelSparkPaths,
	kernelStatusMeta,
} from "#/components/terminal/kernel-meta";
import type {
	TerminalKernel,
	TerminalModel,
} from "#/components/terminal/model";
import { SurfaceBody } from "#/components/terminal/surfaces";

const sampleKernel = (
	overrides: Partial<TerminalKernel> & Pick<TerminalKernel, "source" | "name">,
): TerminalKernel => {
	const status = overrides.status ?? "healthy";
	const statusMeta = kernelStatusMeta(status);
	const spark = kernelSparkPaths([0.42, 0.44, 0.41]);

	return {
		category: overrides.source,
		sub: `${overrides.source} · sample`,
		blurb: "Sample kernel blurb.",
		status,
		statusLabel: statusMeta.label,
		statusMeta,
		strengthText: "0.4000",
		confidencePercent: 42,
		surprisePercent: 50,
		surpriseRaw: 1.8,
		healthPercent: 100,
		confidenceText: "42%",
		surpriseText: "1.80× thr",
		samplesText: "calibrated",
		activeText: "1 / 8 obs",
		observedText: "60000ms · 60.0s ago",
		faultText: "none",
		spark,
		...overrides,
	};
};

const baseModel: TerminalModel = {
	online: true,
	clockText: "13:30:00",
	uptimeText: "0m 0s",
	wallet: {
		cash: "$0.00",
		available: "$0.00",
		reserved: "0.00 EUR",
		tick: "0",
		openText: "0 open positions",
	},
	engine: {
		phase: "stream",
		sequence: "#0",
		measurements: 10,
		quotesReady: 10,
		candidates: 0,
		open: 0,
		signalsText: "10/14",
		signalsPercent: 71,
		fluidText: "3",
		fluidPercent: 37,
		rejectText: "",
		throttled: false,
	},
	entryLineText: "0.000",
	field: {
		gridText: "64 × 38",
		outliersText: "0 outliers",
		peakText: "peak 0.00",
		outliers: 0,
		peak: "0.00",
		focusSymbol: "market",
	},
	prediction: {
		symbol: "market",
		errText: "0.00",
		confText: "0%",
	},
	health: {
		healthy: 2,
		total: 14,
		averageConfidence: 23,
		firing: 1,
		warming: 0,
		degraded: 0,
		label: "Thin",
	},
	kernels: [],
	decisions: [],
	positions: [],
	totalPnlText: "$0.0000",
	totalPnlPositive: true,
	auditRows: [],
	cognitive: null,
	cognitiveScopes: [],
	crossSection: [],
	playbookBranches: 0,
	walkSymbol: "",
	regime: {
		volatility: 0,
		trend: 0,
		bullish: 0,
		bearish: 0,
		choppiness: 0,
	},
};

const model = baseModel;

describe("DashboardSurface", () => {
	it("renders the mockup pulse strip and owned chart panels", () => {
		const html = renderToStaticMarkup(
			<DashboardSurface
				model={model}
				selectedSource="pumpdump"
				inspectorSource={null}
				onInspect={() => undefined}
				onCloseInspect={() => undefined}
				onOpenInsight={() => undefined}
			/>,
		);

		expect(html).toContain("meas 10");
		expect(html).toContain("navier–stokes · vol-rank × Δ · whale carriers");
		expect(html).toContain("whale carrier");
		expect(html).toContain("grid 64×38");
		expect(html).toContain("Predictive coding");
		expect(html).toContain("Comb");
		expect(html).toContain("Edge");
		expect(html).not.toContain("<h1");
	});

	it("renders x-ray and cognitive context strips", () => {
		const xray = renderToStaticMarkup(
			<SurfaceBody
				surface="xray"
				model={model}
				selectedSource="pumpdump"
				inspectorSource={null}
				onSelectKernel={() => undefined}
				onInspectKernel={() => undefined}
				onCloseInspect={() => undefined}
				onOpenInsight={() => undefined}
			/>,
		);
		const cortex = renderToStaticMarkup(
			<SurfaceBody
				surface="cortex"
				model={model}
				selectedSource="pumpdump"
				inspectorSource={null}
				onSelectKernel={() => undefined}
				onInspectKernel={() => undefined}
				onCloseInspect={() => undefined}
				onOpenInsight={() => undefined}
			/>,
		);

		expect(xray).toContain("Inspect symbol");
		expect(xray).toContain("Predictive-coding hierarchy");
		expect(xray).toContain("Manifold reading");
		expect(xray).toContain("momentum eigenmode");
		expect(cortex).toContain("Sensory context");
		expect(cortex).toContain("Beam search lookahead");
	});

	it("renders signal insight with compact kernels and selected detail", () => {
		const signalModel: TerminalModel = {
			...model,
			kernels: [
				sampleKernel({
					source: "causal",
					name: "Causal ladder",
					sub: "causal · assoc→interv→cf",
					blurb:
						"Pearl do-calculus. Climbs association → intervention → counterfactual to estimate the effect of acting, not merely observing.",
				}),
				sampleKernel({
					source: "depthflow",
					name: "Depth flow",
					confidencePercent: 33,
					confidenceText: "33%",
				}),
			],
			decisions: [
				{
					key: "NEAR/EUR:causal",
					symbol: "NEAR/EUR",
					source: "causal",
					scoreText: "0.589",
					scoreValue: 0.589,
					verdict: "blocked",
					why: "below edge",
					edgeText: "+0.089 / 0.500",
					edgePositive: false,
					signals: [{ source: "causal", confidence: 0.589 }],
				},
			],
			crossSection: [
				{
					key: "BTC/EUR:0",
					label: "BTC",
					title: "BTC/EUR 1.000",
					value: 1,
				},
			],
		};
		const html = renderToStaticMarkup(
			<SurfaceBody
				surface="signals"
				model={signalModel}
				selectedSource="causal"
				inspectorSource={null}
				onSelectKernel={() => undefined}
				onInspectKernel={() => undefined}
				onCloseInspect={() => undefined}
				onOpenInsight={() => undefined}
			/>,
		);

		expect(html).toContain("Pearl do-calculus");
		expect(html).toContain("Active readings");
		expect(html).toContain("Cross-section · confidence heatmap");
		expect(html).toContain("BTC");
		expect(html).not.toContain("NEAR");
		expect(html).toContain("Causal ladder");
		expect(html.indexOf("Causal ladder")).toBeLessThan(html.indexOf("Depth flow"));
	});

	it("renders the decision tree candidate surface from backend rows", () => {
		const decisionModel: TerminalModel = {
			...model,
			kernels: [
				sampleKernel({ source: "pumpdump", name: "Pump impulse" }),
				sampleKernel({ source: "causal", name: "Causal ladder" }),
				sampleKernel({ source: "correlation", name: "Correlation field" }),
			],
			decisions: [
				{
					key: "NEAR/EUR:pumpdump",
					symbol: "NEAR/EUR",
					source: "pumpdump",
					scoreText: "0.589",
					scoreValue: 0.589,
					verdict: "blocked",
					why: "below line",
					edgeText: "+0.089 / 0.500",
					edgePositive: false,
					signals: [
						{ source: "pumpdump", confidence: 0.589 },
						{ source: "causal", confidence: 0.237 },
					],
				},
			],
			cognitive: {
				scope: "NEAR/EUR",
				sequence: "Z8RW-77JS-HM3K-245Y-KFY4",
				regimePrefix: "breakout",
				regimeCohort: 9,
				ambiguous: false,
				sideline: false,
				entropyBits: 2.02,
				entropyThreshold: 3.6,
				classConfidence: 0.2,
				contrastEvidence: 0,
				lookaheadScore: 0.763,
				lookaheadPaths: 17,
				winnerClass: "breakout",
				prewarmPaths: null,
				prewarmScore: null,
				updatedAt: 0,
			},
		};
		const html = renderToStaticMarkup(
			<SurfaceBody
				surface="decisions"
				model={decisionModel}
				selectedSource="pumpdump"
				inspectorSource={null}
				onSelectKernel={() => undefined}
				onInspectKernel={() => undefined}
				onCloseInspect={() => undefined}
				onOpenInsight={() => undefined}
			/>,
		);

		expect(html).toContain("Candidate evaluation");
		expect(html).toContain("universe");
		expect(html).toContain("NEAR/EUR");
		expect(html).toContain("Score attribution");
		expect(html).toContain("Playbook walk");
		expect(html).toContain("Causal ladder");
		expect(html).toContain("Cognitive beam");
		expect(html).not.toContain("Open positions");
	});
});
