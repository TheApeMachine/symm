// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookAtom } from "#/collections/book";
import {
	type BookDeskStats,
	type LongtailSample,
	longtailStore,
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

const bookStats = (overrides: Partial<BookDeskStats> = {}): BookDeskStats => ({
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
	tokenEmitRate: null,
	blockedRate: null,
	blockedBrightnessRate: null,
	blockedDuplicateRate: null,
	vetoRate: null,
	emptyRate: null,
	policyStoredRate: null,
	outcomeRate: null,
	outcomeEnterRate: null,
	outcomeExitRate: null,
	violations: null,
	exploredEnter: null,
	venueOpen: null,
	virtualExcursion: null,
	developedPaths: null,
	promotedPaths: null,
	prunedPaths: null,
	meanThreshold: null,
	exitProfit: null,
	exitStop: null,
	exitBranch: null,
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
					virtual: bookStats({
						closed: 4,
						wins: 3,
						winRate: 0.6,
						meanReturn: 0.01,
						avgHoldNs: 120e9,
					}),
					training: training({
						feeds: 2,
						tokenEmitRate: 0.08,
						developedPaths: 6,
						promotedPaths: 1,
						prunedPaths: 0,
						meanThreshold: 0.4,
						exitProfit: 2,
						exitStop: 1,
						exitBranch: 1,
					}),
				}),
				sample(6_000, {
					virtual: bookStats({
						closed: 5,
						wins: 4,
						winRate: 0.65,
						meanReturn: 0.03,
						avgHoldNs: 180e9,
					}),
					training: training({
						feeds: 2,
						tokenEmitRate: 0.1,
						developedPaths: 8,
						promotedPaths: 2,
						prunedPaths: 0,
						meanThreshold: 0.35,
						exitProfit: 3,
						exitStop: 1,
						exitBranch: 1,
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
			"hold",
			"letters",
			"developed",
			"promoted",
			"pruned",
			"branch",
		]) {
			expect(container.querySelector(`[data-kpi="${id}"]`)).not.toBeNull();
		}

		for (const id of ["win", "return", "hold", "letters", "paths", "exits"]) {
			expect(container.querySelector(`[data-chart="${id}"]`)).not.toBeNull();
		}
		expect(container.querySelectorAll("svg[role='img']")).toHaveLength(6);

		for (const title of ["Paths", "Letters", "Desk", "Exits"]) {
			expect(getByText(title, { selector: "div" })).toBeTruthy();
		}
		expect(getByText("Plain-language readings")).toBeTruthy();
	});

	it("paints a bad edge red and a good edge green — never the reverse", () => {
		const bad = [
			sample(1_000, {
				virtual: bookStats({
					closed: 5,
					wins: 1,
					winRate: 0.2,
					meanReturn: -0.1,
				}),
				training: training({
					rules: 3,
					reuseRate: 0.1,
					createRate: 0.8,
					tokenEmitRate: 0.03,
					policyStoredRate: 0.01,
				}),
			}),
			sample(6_000, {
				virtual: bookStats({
					closed: 6,
					wins: 1,
					winRate: 0.15,
					meanReturn: -0.12,
				}),
				training: training({
					rules: 3,
					reuseRate: 0.1,
					createRate: 0.8,
					tokenEmitRate: 0.03,
					policyStoredRate: 0.01,
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
				virtual: bookStats({
					closed: 4,
					wins: 3,
					winRate: 0.6,
					meanReturn: 0.01,
				}),
				training: training({
					rules: 10,
					reuseRate: 0.6,
					createRate: 0.3,
					tokenEmitRate: 0.08,
					policyStoredRate: 0.05,
				}),
			}),
			sample(6_000, {
				virtual: bookStats({
					closed: 5,
					wins: 4,
					winRate: 0.65,
					meanReturn: 0.03,
				}),
				training: training({
					rules: 11,
					reuseRate: 0.6,
					createRate: 0.3,
					tokenEmitRate: 0.08,
					policyStoredRate: 0.05,
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

	it("paints a short hold red and a long hold green", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					virtual: bookStats({
						closed: 2,
						wins: 1,
						winRate: 0.5,
						meanReturn: 0.01,
						avgHoldNs: 10e9,
					}),
				}),
				sample(6_000, {
					virtual: bookStats({
						closed: 2,
						wins: 1,
						winRate: 0.5,
						meanReturn: 0.01,
						avgHoldNs: 10e9,
					}),
				}),
			],
			lastSampleAt: 6_000,
		}));
		bookAtom.set("virtual");
		const shortView = render(<LongtailDashboard />);
		const shortHold = shortView.container.querySelector(
			'[data-kpi="hold"] > span',
		) as HTMLElement;
		expect(shortHold.style.color).toBe("var(--down)");
		expect(shortHold.textContent).toContain("10s");

		cleanup();

		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					virtual: bookStats({
						closed: 2,
						wins: 1,
						winRate: 0.5,
						meanReturn: 0.01,
						avgHoldNs: 600e9,
					}),
				}),
				sample(6_000, {
					virtual: bookStats({
						closed: 2,
						wins: 1,
						winRate: 0.5,
						meanReturn: 0.01,
						avgHoldNs: 600e9,
					}),
				}),
			],
			lastSampleAt: 6_000,
		}));
		const longView = render(<LongtailDashboard />);
		const longHold = longView.container.querySelector(
			'[data-kpi="hold"] > span',
		) as HTMLElement;
		expect(longHold.style.color).toBe("var(--up)");
		expect(longHold.textContent).toContain("10m");
	});

	it("reads the selected book's metrics", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					paper: bookStats({
						closed: 2,
						wins: 2,
						winRate: 1,
						meanReturn: 0.05,
					}),
					virtual: bookStats({
						closed: 2,
						wins: 0,
						winRate: 0,
						meanReturn: -0.05,
					}),
				}),
				sample(6_000, {
					paper: bookStats({
						closed: 2,
						wins: 2,
						winRate: 1,
						meanReturn: 0.05,
					}),
					virtual: bookStats({
						closed: 2,
						wins: 0,
						winRate: 0,
						meanReturn: -0.05,
					}),
				}),
			],
			lastSampleAt: 6_000,
		}));
		bookAtom.set("paper");

		const { container } = render(<LongtailDashboard />);
		const edge = container.querySelector(
			'[data-kpi="edge"] .font-bold',
		) as HTMLElement;
		expect(edge.textContent).toBe("+5.0%");
		expect(edge.style.color).toBe("var(--up)");
	});
});
