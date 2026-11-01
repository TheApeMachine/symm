// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookAtom } from "#/collections/book";
import {
	type BookDeskStats,
	type ExitMix,
	emptyExitMix,
	emptyPaths,
	emptyPolicy,
	emptyStops,
	type LongtailSample,
	longtailStore,
	type OpenLotTelemetry,
	type PathBookSnapshot,
	type PolicySnapshot,
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

const sample = (
	at: number,
	overrides: Partial<{
		paper: Partial<BookDeskStats>;
		virtual: Partial<BookDeskStats>;
		real: Partial<BookDeskStats>;
		paperWin: number | null;
		paperEdge: number | null;
		realWin: number | null;
		realEdge: number | null;
		realArmed: number | null;
		paths: Partial<PathBookSnapshot>;
		policy: Partial<PolicySnapshot>;
		exits: Partial<Record<"virtual" | "paper" | "real", Partial<ExitMix>>>;
		stops: Partial<
			Record<"virtual" | "paper" | "real", Partial<OpenLotTelemetry>>
		>;
	}> = {},
): LongtailSample => ({
	at,
	paper: bookStats(overrides.paper),
	virtual: bookStats(overrides.virtual),
	real: bookStats(overrides.real),
	paperWin: overrides.paperWin ?? null,
	paperEdge: overrides.paperEdge ?? null,
	realWin: overrides.realWin ?? null,
	realEdge: overrides.realEdge ?? null,
	realArmed: overrides.realArmed ?? null,
	eligibleResistance: 0.2,
	paths: { ...emptyPaths(), ...overrides.paths },
	policy: { ...emptyPolicy(), ...overrides.policy },
	exits: {
		virtual: { ...emptyExitMix(), ...overrides.exits?.virtual },
		paper: { ...emptyExitMix(), ...overrides.exits?.paper },
		real: { ...emptyExitMix(), ...overrides.exits?.real },
	},
	stops: {
		virtual: { ...emptyStops(), ...overrides.stops?.virtual },
		paper: { ...emptyStops(), ...overrides.stops?.paper },
		real: { ...emptyStops(), ...overrides.stops?.real },
	},
});

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	longtailStore.setState(() => ({
		samples: [],
		lastSampleAt: 0,
		sourcesReady: false,
	}));
});

describe("LongtailDashboard", () => {
	it("reports warming up and waits for samples with an empty window", () => {
		const { getByText, container } = render(<LongtailDashboard />);

		expect(getByText("Longtail")).toBeTruthy();
		expect(getByText("Warming up")).toBeTruthy();
		expect(container.textContent).toContain("waiting for samples…");
	});

	it("renders the KPI strip, charts, and the right rail for the selected book", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					virtual: bookStats({
						closed: 4,
						wins: 3,
						winRate: 0.6,
						meanReturn: 0.01,
					}),
					paths: {
						developed: 6,
						promoted: 1,
						meanResistance: 0.4,
						added: 1,
						removed: 0,
					},
					policy: { checks: 4, hits: 2, hitRate: 0.5 },
					exits: {
						virtual: { profit: 2, stop: 1, policy: 1, other: 0, total: 4 },
					},
				}),
				sample(6_000, {
					virtual: bookStats({
						closed: 5,
						wins: 4,
						winRate: 0.65,
						meanReturn: 0.03,
					}),
					paths: {
						developed: 8,
						promoted: 2,
						meanResistance: 0.35,
						added: 2,
						removed: 0,
					},
					policy: { checks: 4, hits: 3, hitRate: 0.75 },
					exits: {
						virtual: { profit: 3, stop: 1, policy: 1, other: 0, total: 5 },
					},
				}),
			],
			lastSampleAt: 6_000,
			sourcesReady: true,
		}));
		bookAtom.set("virtual");

		const { container, getByText } = render(<LongtailDashboard />);

		for (const id of [
			"win",
			"edge",
			"closed",
			"promoted",
			"resistance",
			"policy",
		]) {
			expect(container.querySelector(`[data-kpi="${id}"]`)).not.toBeNull();
		}

		for (const id of ["edge", "paths", "churn", "policy", "exits", "stops"]) {
			expect(container.querySelector(`[data-chart="${id}"]`)).not.toBeNull();
		}
		expect(container.querySelectorAll("[data-chart]")).toHaveLength(6);

		expect(getByText("virtual book", { selector: "div" })).toBeTruthy();
		expect(getByText("Paths", { selector: "div" })).toBeTruthy();
		expect(getByText("Policy", { selector: "div" })).toBeTruthy();
		expect(getByText("Exits", { selector: "div" })).toBeTruthy();
		expect(getByText("Plain-language readings")).toBeTruthy();
		expect(getByText("Improving")).toBeTruthy();
	});

	it("scopes KPI values to the selected tier", () => {
		longtailStore.setState(() => ({
			samples: [
				sample(1_000, {
					paper: bookStats({ winRate: 0.4, meanReturn: -0.02, closed: 5 }),
					paperWin: 0.4,
					paperEdge: -0.02,
					real: bookStats({ winRate: 0.7, meanReturn: 0.04, closed: 3 }),
					realWin: 0.7,
					realEdge: 0.04,
					realArmed: 1,
				}),
				sample(6_000, {
					paper: bookStats({ winRate: 0.42, meanReturn: -0.01, closed: 6 }),
					paperWin: 0.42,
					paperEdge: -0.01,
					real: bookStats({ winRate: 0.75, meanReturn: 0.05, closed: 4 }),
					realWin: 0.75,
					realEdge: 0.05,
					realArmed: 1,
				}),
			],
			lastSampleAt: 6_000,
			sourcesReady: true,
		}));

		bookAtom.set("paper");
		const paperView = render(<LongtailDashboard />);
		const paperWin = paperView.container.querySelector(
			'[data-kpi="win"] .font-bold',
		) as HTMLElement;
		expect(paperWin.textContent).toBe("42%");
		cleanup();

		bookAtom.set("real");
		const realView = render(<LongtailDashboard />);
		const realWin = realView.container.querySelector(
			'[data-kpi="win"] .font-bold',
		) as HTMLElement;
		expect(realWin.textContent).toBe("75%");
		expect(realView.getByText("Improving")).toBeTruthy();
	});

	it("paints weak policy hits red and strong hits green", () => {
		const bad = [
			sample(1_000, { policy: { checks: 10, hits: 0, hitRate: 0.01 } }),
			sample(6_000, { policy: { checks: 10, hits: 0, hitRate: 0.01 } }),
		];

		longtailStore.setState(() => ({
			samples: bad,
			lastSampleAt: 6_000,
			sourcesReady: true,
		}));
		const badView = render(<LongtailDashboard />);

		const badPolicy = badView.container.querySelector(
			'[data-kpi="policy"] .font-bold',
		) as HTMLElement;
		expect(badPolicy.style.color).toBe("var(--down)");

		cleanup();

		const good = [
			sample(1_000, { policy: { checks: 10, hits: 6, hitRate: 0.55 } }),
			sample(6_000, { policy: { checks: 10, hits: 6, hitRate: 0.6 } }),
		];

		longtailStore.setState(() => ({
			samples: good,
			lastSampleAt: 6_000,
			sourcesReady: true,
		}));
		const goodView = render(<LongtailDashboard />);

		const goodPolicy = goodView.container.querySelector(
			'[data-kpi="policy"] .font-bold',
		) as HTMLElement;
		expect(goodPolicy.style.color).toBe("var(--up)");
	});
});
