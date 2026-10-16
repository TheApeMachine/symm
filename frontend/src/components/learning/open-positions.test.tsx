// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { OpenPositions } from "./open-positions";

const createValue = (values: Record<string, number>) => {
	return new Value({
		origin: "training",
		scope: "RLC/USD",
		metrics: Object.entries(values).map(([name, raw]) => ({
			name,
			raw,
		})),
	});
};

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
});

describe("OpenPositions", () => {
	it("shows the empty state when nothing is held", () => {
		const ring = new RingBuffer<Value>(8);
		ring.add(createValue({ position_open: 0 }));
		signals.training.setState(() => ({ "RLC/USD": ring }));
		const view = render(<OpenPositions />);
		expect(view.getByText("No open positions")).toBeTruthy();
	});

	it("draws a card from the position metrics the backend sent", () => {
		const ring = new RingBuffer<Value>(8);
		signals.training.setState(() => ({ "RLC/USD": ring }));
		const view = render(<OpenPositions />);
		act(() => {
			ring.add(
				createValue({
					position_open: 1,
					position_entry: 1,
					position_bid: 1.1,
					position_pnl_pct: 0.05,
					position_exit_room: 4,
					position_held_s: 75,
				}),
			);
			signals.training.setState((s) => ({ ...s }));
		});
		expect(view.getByText("RLC/USD")).toBeTruthy();
		expect(view.getByText("held 1m 15s")).toBeTruthy();
		expect(view.getByText("4.0×")).toBeTruthy();
		expect(view.getByText("5.0%")).toBeTruthy();
		expect(view.getByText("waiting for tokens")).toBeTruthy();
	});
});
