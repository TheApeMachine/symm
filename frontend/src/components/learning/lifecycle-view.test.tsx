// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LifecycleReport } from "#/lib/lifecycle";
import { LifecycleView } from "./lifecycle-view";

const report: LifecycleReport = {
	lifecycles: [
		{
			id: "a",
			symbol: "SUI/USD",
			status: "closed",
			opened_at: "2026-10-10T08:00:00Z",
			closed_at: "2026-10-10T08:03:00Z",
			events: [
				{
					at: "2026-10-10T08:00:00Z",
					kind: "entry_match",
					detail: "learned enter matched on R01/R02/R03",
					fields: {
						path: "R01/R02/R03",
						candidates: 2,
						confidence: 3,
						threshold: 3,
					},
				},
				{
					at: "2026-10-10T08:00:00Z",
					kind: "sizing",
					detail: "sized 12.5 bound by exit_capacity",
					fields: { binding: "exit_capacity", exit_capacity: 12.5, cash: 200 },
				},
				{
					at: "2026-10-10T08:01:00Z",
					kind: "risk_sell",
					detail: "capacity_trim sells 4",
					fields: { trigger: "capacity_trim", capacity: 8.5 },
				},
			],
			outcome: {
				venue_cost: 100,
				venue_proceeds: 101,
				venue_fees: 1.6,
				venue_realized: -0.6,
				shadow_defined: true,
				shadow_cost: 100,
				shadow_proceeds: 100.5,
				shadow_fees: 1.6,
				shadow_realized: -1.1,
				shadow_short: 0,
				shadow_unpriced: 0,
				hold_ns: 180e9,
				expected_hold_ns: 120e9,
				triggers: ["capacity_trim", "learned_exit"],
			},
		},
	],
	performance: {
		closed: 1,
		shadow_trades: 1,
		shadow_wins: 0,
		shadow_win_rate: 0,
		shadow_mean_return: -0.011,
		shadow_pnl: -1.1,
		venue_wins: 0,
		venue_win_rate: 0,
		venue_mean_return: -0.006,
		venue_pnl: -0.6,
	},
};

afterEach(() => {
	cleanup();
	vi.unstubAllGlobals();
});

describe("LifecycleView", () => {
	it("renders the served timeline, the outcome and the session performance", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => new Response(JSON.stringify(report), { status: 200 })),
		);

		const { container } = render(<LifecycleView />);
		await act(async () => {
			await new Promise((resolve) => setTimeout(resolve, 0));
		});

		const kinds = [...container.querySelectorAll("[data-lifecycle-event]")].map(
			(el) => el.getAttribute("data-lifecycle-event"),
		);
		expect(kinds).toEqual(["entry_match", "sizing", "risk_sell"]);
		expect(container.textContent).toContain("R01/R02/R03");
		expect(container.textContent).toContain("capacity_trim, learned_exit");
		expect(
			container.querySelector('[data-l="lifecycle-outcome"]')?.textContent,
		).toContain("-1.10");
		expect(
			container.querySelector('[data-l="lifecycle-performance"]')?.textContent,
		).toContain("shadow win 0.0%");
	});
});
