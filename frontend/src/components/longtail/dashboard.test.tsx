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

		expect(getByText("Path learning")).toBeTruthy();
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
			"reuse",
			"create",
			"policy",
			"coverage",
			"letters",
			"developed",
			"promoted",
			"explore",
		]) {
			expect(container.querySelector(`[data-kpi="${id}"]`)).not.toBeNull();
		}

		for (const id of [
			"grammar",
			"policy",
			"coverage",
			"letters",
			"paths",
			"exits",
		]) {
			expect(container.querySelector(`[data-chart="${id}"]`)).not.toBeNull();
		}
		expect(container.querySelectorAll("[data-chart]")).toHaveLength(6);

		for (const title of ["Paths", "Grammar", "Letters", "Desk", "Exits"]) {
			expect(getByText(title, { selector: "div" })).toBeTruthy();
		}
		expect(getByText("Plain-language readings")).toBeTruthy();
	});

	it("paints low reuse red and high reuse green", () => {
		const bad = [
			sample(1_000, {
				training: training({
					reuseRate: 0.1,
					createRate: 0.8,
					policyStoredRate: 0.01,
				}),
			}),
			sample(6_000, {
				training: training({
					reuseRate: 0.1,
					createRate: 0.8,
					policyStoredRate: 0.01,
				}),
			}),
		];

		longtailStore.setState(() => ({ samples: bad, lastSampleAt: 6_000 }));
		const badView = render(<LongtailDashboard />);

		const badReuse = badView.container.querySelector(
			'[data-kpi="reuse"] .font-bold',
		) as HTMLElement;
		expect(badReuse.style.color).toBe("var(--down)");
		expect(badView.getByText("Finding structure")).toBeTruthy();

		cleanup();

		const good = [
			sample(1_000, {
				training: training({
					reuseRate: 0.6,
					createRate: 0.2,
					policyStoredRate: 0.4,
				}),
			}),
			sample(6_000, {
				training: training({
					reuseRate: 0.65,
					createRate: 0.18,
					policyStoredRate: 0.42,
				}),
			}),
		];

		longtailStore.setState(() => ({ samples: good, lastSampleAt: 6_000 }));
		const goodView = render(<LongtailDashboard />);

		const goodReuse = goodView.container.querySelector(
			'[data-kpi="reuse"] .font-bold',
		) as HTMLElement;
		expect(goodReuse.style.color).toBe("var(--up)");
		expect(goodView.getByText("Reusing paths")).toBeTruthy();
	});
});
