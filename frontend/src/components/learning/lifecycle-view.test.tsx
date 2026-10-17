// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ingestPositionFrame, resetLifecycles } from "#/lib/lifecycle";
import { LifecycleView } from "./lifecycle-view";

const base = {
	id: "SUI/USD@1",
	symbol: "SUI/USD",
	policy_epoch: "7",
	opened_at: "2026-10-10T08:00:00Z",
	closed_at: null,
	mark: null,
	outcome: null,
};

const event = (
	status: string,
	kind: string,
	detail: string,
	fields?: Record<string, unknown>,
) => ({
	...base,
	status,
	event: { at: "2026-10-10T08:00:00Z", kind, detail, fields },
});

const entered = [
	event("entering", "entry_match", "policy enter match R01/R02/R03", {
		path: "R01/R02/R03",
		depth: 3,
		confidence: 0.66,
	}),
	event("entering", "sizing", "sized 12.5 at the ask 8", {
		cash: 1000,
		allocated: 100,
		quantity: 12.5,
		reference: 8,
	}),
	event("entering", "order_submitted", "buy 12.5 submitted to the paper desk", {
		side: "buy",
		submitted: 12.5,
	}),
	event("holding", "fill", "buy 12.5 at 8 (12.5 of 12.5 filled)", {
		quantity: 12.5,
		price: 8,
		cost: 100,
		fee: 0.1,
		filled: 12.5,
		ordered: 12.5,
		complete: true,
	}),
];

const mark = {
	...base,
	status: "holding",
	event: null,
	mark: {
		at: "2026-10-10T08:01:00Z",
		bid: 8.2,
		quantity: 12.5,
		entry_price: 8,
		unrealized: 2.4,
		unrealized_return: 0.024,
	},
};

const deliver = (frames: unknown[]) =>
	act(() => {
		for (const frame of frames) ingestPositionFrame(frame);
	});

afterEach(() => {
	cleanup();
	resetLifecycles();
});

describe("LifecycleView", () => {
	it("renders an open paper position from hub frames, marked at the live bid", () => {
		const { container } = render(<LifecycleView />);
		expect(container.textContent).toContain("No positions this session yet.");

		deliver([...entered, mark]);

		const kinds = [...container.querySelectorAll("[data-lifecycle-event]")].map(
			(el) => el.getAttribute("data-lifecycle-event"),
		);
		expect(kinds).toEqual(["entry_match", "sizing", "order_submitted", "fill"]);
		expect(container.textContent).toContain("SUI/USD · holding");
		expect(
			container.querySelector('[data-l="lifecycle-mark"]')?.textContent,
		).toContain("+2.40 USD unrealized");
		expect(container.textContent).toContain("R01/R02/R03");
	});

	it("renders the close with its trigger, and exchange figures as not available", () => {
		const { container } = render(<LifecycleView />);
		deliver([
			...entered,
			mark,
			event("exiting", "exit_match", "policy exit match R02/R03 (depth 2)", {
				trigger: "policy exit match R02/R03 (depth 2)",
			}),
			{
				...event("closed", "closed", "closed: pnl -1.100000"),
				closed_at: "2026-10-10T08:03:00Z",
				outcome: {
					cost: 100.1,
					proceeds: 99,
					fees: 0.2,
					realized: -1.1,
					net_return: -0.011,
					hold_ns: 180e9,
					expected_hold_ns: 120e9,
					triggers: ["policy exit match R02/R03 (depth 2)"],
				},
			},
		]);

		const outcome = container.querySelector('[data-l="lifecycle-outcome"]');
		expect(outcome?.textContent).toContain("-1.10");
		expect(outcome?.textContent).toContain("exchange P&Lnot available");
		expect(outcome?.textContent).toContain("3.0m / 2.0m");
		expect(outcome?.textContent).toContain(
			"policy exit match R02/R03 (depth 2)",
		);

		const performance = container.querySelector(
			'[data-l="lifecycle-performance"]',
		)?.textContent;
		expect(performance).toContain("1 closed");
		expect(performance).toContain("paper win 0.0%");
		expect(performance).toContain("exchange win · edge · P&L not available");
		expect(container.textContent).not.toMatch(/exchange[^·]*\$?0\.00/);
	});

	it("draws only the sizing inputs the event carries", () => {
		const { container } = render(<LifecycleView />);
		deliver(entered);

		const sizing = container.querySelector(
			'[data-lifecycle-event="sizing"] button',
		) as HTMLButtonElement;
		act(() => sizing.click());

		const widget = container.querySelector('[data-l="lifecycle-sizing"]');
		expect(widget?.textContent).toContain("Cash allocated");
		expect(widget?.textContent).toContain("100.00 USD");
		expect(widget?.textContent).not.toContain("Book Exit Depth");
		expect(widget?.textContent).not.toContain("Flow Participation");
		expect(widget?.textContent).not.toContain("Bound by");
	});
});
