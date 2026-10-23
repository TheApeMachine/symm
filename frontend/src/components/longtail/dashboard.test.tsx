// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookAtom } from "#/collections/book";
import {
	longtailStore,
	type BookDeskStats,
	type LongtailSample,
	type TrainingSample,
} from "#/collections/longtail";
import { LongtailDashboard } from "./dashboard";

/*
jsdom has no ResizeObserver; the stub reports a fixed cell size so the
measured charts draw instead of waiting for a layout that never happens.
*/
class ResizeObserverStub {
	callback: ResizeObserverCallback;

	constructor(callback: ResizeObserverCallback) {
		this.callback = callback;
	}

	observe(element: Element) {
		this.callback(
			[
				{
					contentRect: { width: 300, height: 200 },
					target: element,
				} as unknown as ResizeObserverEntry,
			],
			this as unknown as ResizeObserver,
		);
	}

	unobserve() {}
	disconnect() {}
}

vi.stubGlobal("ResizeObserver", ResizeObserverStub);

const bookStats = (
	overrides: Partial<BookDeskStats> = {},
): BookDeskStats => ({
	closed: 0,
	open: 0,
	wins: 0,
	winRate: null,
	meanReturn: null,
	medianReturn: null,
	worstReturn: null,
	avgHoldNs: null,
	...overrides,
});

const training = (overrides: Partial<TrainingSample> = {}): TrainingSample => ({
	feeds: 0,
	rules: null,
	grammarSymbols: null,
	rawLength: null,
	avgRuleDepth: null,
	maxRuleDepth: null,
	actionCoverage: null,
	createRate: null,
	reuseRate: null,
	inlineRate: null,
	gatePassRate: null,
	blockedRate: null,
	warmRate: null,
	avgSurprise: null,
	avgZ: null,
	policyStoredRate: null,
	policyMatchRate: null,
	avgMatchDepth: null,
	outcomeRate: null,
	violations: null,
	...overrides,
});

const sample = (
	at: number,
	overrides: Partial<{
		paper: Partial<BookDeskStats>;
		virtual: Partial<BookDeskStats>;
		training: Partial<TrainingSample>;
	}> = {},
): LongtailSample => ({
	at,
	paper: bookStats(overrides.paper),
	virtual: bookStats(overrides.virtual),
	training: training(overrides.training),
});

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	longtailStore.setState(() => ({ samples: [], lastSampleAt: 0 }));
});

describe("LongtailDashboard", () => {
	it("reports warming up and waits for history with no samples", () => {
		const { getByText, container } = render(<LongtailDashboard />);

		expect(getByText("Training performance")).toBeTruthy();
		expect(getByText("Warming up")).toBeTruthy();
		// Every chart waits for a second sample before it can draw a line.
		expect(container.textContent).toContain("collecting history…");
	});

	it("renders the KPI strip, the six charts, and the right rail", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					virtual: bookStats({ closed: 4, wins: 3, winRate: 0.6, meanReturn: 0.01 }),
					training: training({
						feeds: 2,
						rules: 10,
						grammarSymbols: 40,
						rawLength: 100,
						avgRuleDepth: 2,
						maxRuleDepth: 4,
						actionCoverage: 0.6,
						reuseRate: 0.6,
						createRate: 0.3,
						gatePassRate: 0.8,
						warmRate: 0.8,
						avgSurprise: 0.4,
						avgZ: 1,
						policyStoredRate: 0.5,
						policyMatchRate: 0.6,
						avgMatchDepth: 3,
						outcomeRate: 0.2,
					}),
				}),
				sample(6_000, {
					virtual: bookStats({ closed: 5, wins: 4, winRate: 0.65, meanReturn: 0.03 }),
					training: training({
						feeds: 2,
						rules: 11,
						grammarSymbols: 44,
						rawLength: 110,
						avgRuleDepth: 2,
						maxRuleDepth: 4,
						actionCoverage: 0.6,
						reuseRate: 0.6,
						createRate: 0.3,
						gatePassRate: 0.8,
						warmRate: 0.8,
						avgSurprise: 0.4,
						avgZ: 1,
						policyStoredRate: 0.5,
						policyMatchRate: 0.6,
						avgMatchDepth: 3,
						outcomeRate: 0.2,
					}),
				}),
			],
			lastSampleAt: 6_000,
		}));
		bookAtom.set("virtual");

		const { container, getByText } = render(<LongtailDashboard />);

		// Eight KPI tiles.
		for (const id of [
			"edge",
			"win",
			"coverage",
			"reuse",
			"gate",
			"surprise",
			"match",
			"warm",
		]) {
			expect(container.querySelector(`[data-kpi="${id}"]`)).not.toBeNull();
		}

		// Six measured charts.
		for (const id of ["win", "return", "gate", "surprise", "reuse", "match"]) {
			expect(container.querySelector(`[data-chart="${id}"]`)).not.toBeNull();
		}
		expect(container.querySelectorAll("svg[aria-label='metric chart']")).toHaveLength(6);

		// Right rail blocks.
		expect(getByText("Grammar")).toBeTruthy();
		expect(getByText("Surprise gate")).toBeTruthy();
		expect(getByText("Policy")).toBeTruthy();
		expect(getByText("Plain-language readings")).toBeTruthy();
	});

	it("paints a bad edge red and a good edge green — never the reverse", () => {
		const bad = [
			sample(1_000, {
				virtual: bookStats({ closed: 5, wins: 1, winRate: 0.2, meanReturn: -0.1 }),
				training: training({
					rules: 3,
					reuseRate: 0.1,
					createRate: 0.8,
					gatePassRate: 0.3,
					policyStoredRate: 0.5,
					policyMatchRate: 0.1,
				}),
			}),
			sample(6_000, {
				virtual: bookStats({ closed: 6, wins: 1, winRate: 0.15, meanReturn: -0.12 }),
				training: training({
					rules: 3,
					reuseRate: 0.1,
					createRate: 0.8,
					gatePassRate: 0.3,
					policyStoredRate: 0.5,
					policyMatchRate: 0.1,
				}),
			}),
		];

		longtailStore.setState(() => ({ samples: bad, lastSampleAt: 6_000 }));
		bookAtom.set("virtual");
		const badView = render(<LongtailDashboard />);

		const badEdge = badView.container.querySelector(
			'[data-kpi="edge"] .tabular-nums',
		) as HTMLElement;
		expect(badEdge.style.color).toBe("var(--down)");
		expect(badView.getByText("Degrading")).toBeTruthy();

		cleanup();

		const good = [
			sample(1_000, {
				virtual: bookStats({ closed: 4, wins: 3, winRate: 0.6, meanReturn: 0.01 }),
				training: training({
					rules: 10,
					reuseRate: 0.6,
					createRate: 0.3,
					gatePassRate: 0.8,
					policyStoredRate: 0.5,
					policyMatchRate: 0.6,
				}),
			}),
			sample(6_000, {
				virtual: bookStats({ closed: 5, wins: 4, winRate: 0.65, meanReturn: 0.03 }),
				training: training({
					rules: 11,
					reuseRate: 0.6,
					createRate: 0.3,
					gatePassRate: 0.8,
					policyStoredRate: 0.5,
					policyMatchRate: 0.6,
				}),
			}),
		];

		longtailStore.setState(() => ({ samples: good, lastSampleAt: 6_000 }));
		const goodView = render(<LongtailDashboard />);

		const goodEdge = goodView.container.querySelector(
			'[data-kpi="edge"] .tabular-nums',
		) as HTMLElement;
		expect(goodEdge.style.color).toBe("var(--up)");
		expect(goodView.getByText("Improving")).toBeTruthy();
	});

	it("paints the surprise chart by its inverted scale: low surprise is green, high surprise is red", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, { training: training({ avgSurprise: 0.4 }) }),
				sample(6_000, { training: training({ avgSurprise: 0.4 }) }),
			],
			lastSampleAt: 6_000,
		}));
		const lowView = render(<LongtailDashboard />);
		const lowChart = lowView.container.querySelector(
			'[data-chart="surprise"] svg[aria-label="metric chart"] path',
		) as SVGPathElement;
		expect(lowChart.getAttribute("stroke")).toBe("var(--up)");

		cleanup();

		longtailStore.setState(() => ({
			samples: [
				sample(1_000, { training: training({ avgSurprise: 3 }) }),
				sample(6_000, { training: training({ avgSurprise: 3 }) }),
			],
			lastSampleAt: 6_000,
		}));
		const highView = render(<LongtailDashboard />);
		const highChart = highView.container.querySelector(
			'[data-chart="surprise"] svg[aria-label="metric chart"] path',
		) as SVGPathElement;
		expect(highChart.getAttribute("stroke")).toBe("var(--down)");
	});

	it("reads the selected book's metrics", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					paper: bookStats({ closed: 2, wins: 2, winRate: 1, meanReturn: 0.05 }),
					virtual: bookStats({ closed: 2, wins: 0, winRate: 0, meanReturn: -0.05 }),
				}),
				sample(6_000, {
					paper: bookStats({ closed: 2, wins: 2, winRate: 1, meanReturn: 0.05 }),
					virtual: bookStats({ closed: 2, wins: 0, winRate: 0, meanReturn: -0.05 }),
				}),
			],
			lastSampleAt: 6_000,
		}));
		bookAtom.set("paper");

		const { container } = render(<LongtailDashboard />);
		const edge = container.querySelector(
			'[data-kpi="edge"] .tabular-nums',
		) as HTMLElement;
		expect(edge.textContent).toBe("+5.0%");
		expect(edge.style.color).toBe("var(--up)");
	});
});
