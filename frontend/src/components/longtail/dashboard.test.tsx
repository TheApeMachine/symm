// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookAtom } from "#/collections/book";
import {
	type BookDeskStats,
	type LongtailSample,
	type OpenLotTelemetry,
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

const emptyStops: OpenLotTelemetry = {
	open: null,
	invalidation: null,
	evaporation: null,
	trailing: null,
	expiry: null,
	armed: null,
	maxProximity: null,
	exits: {},
	exitTotal: 0,
};

const sample = (
	at: number,
	overrides: Partial<{
		paper: Partial<BookDeskStats>;
		virtual: Partial<BookDeskStats>;
		training: Partial<TrainingSample>;
		stops: Partial<OpenLotTelemetry>;
	}> = {},
): LongtailSample => ({
	at,
	paper: bookStats(overrides.paper),
	virtual: bookStats(overrides.virtual),
	training: training(overrides.training),
	stops: { ...emptyStops, ...overrides.stops },
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

	it("renders the KPI strip, charts, and the right rail", () => {
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

		for (const id of [
			"policy",
			"letters",
			"developed",
			"promoted",
			"explore",
		]) {
			expect(container.querySelector(`[data-kpi="${id}"]`)).not.toBeNull();
		}

		for (const id of ["policy", "letters", "paths", "exits", "stops"]) {
			expect(container.querySelector(`[data-chart="${id}"]`)).not.toBeNull();
		}
		expect(container.querySelectorAll("[data-chart]")).toHaveLength(5);

		for (const title of ["Paths", "Policy", "Letters", "Desk", "Exits"]) {
			expect(getByText(title, { selector: "div" })).toBeTruthy();
		}
		expect(getByText("Plain-language readings")).toBeTruthy();
	});

	it("paints weak policy hits red and strong hits green", () => {
		const bad = [
			sample(1_000, {
				training: training({ policyStoredRate: 0.01 }),
			}),
			sample(6_000, {
				training: training({ policyStoredRate: 0.01 }),
			}),
		];

		longtailStore.setState(() => ({ samples: bad, lastSampleAt: 6_000 }));
		const badView = render(<LongtailDashboard />);

		const badPolicy = badView.container.querySelector(
			'[data-kpi="policy"] .font-bold',
		) as HTMLElement;
		expect(badPolicy.style.color).toBe("var(--down)");
		expect(badView.getByText("Learning")).toBeTruthy();

		cleanup();

		const good = [
			sample(1_000, {
				training: training({ policyStoredRate: 0.55 }),
			}),
			sample(6_000, {
				training: training({ policyStoredRate: 0.6 }),
			}),
		];

		longtailStore.setState(() => ({ samples: good, lastSampleAt: 6_000 }));
		const goodView = render(<LongtailDashboard />);

		const goodPolicy = goodView.container.querySelector(
			'[data-kpi="policy"] .font-bold',
		) as HTMLElement;
		expect(goodPolicy.style.color).toBe("var(--up)");
		expect(goodView.getByText("Matching paths")).toBeTruthy();
	});
});
