import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import type { DeskPosition } from "#/collections/types";
import { Value } from "#/types/value";
import { Positions } from "./positions";

/*
Positions reads the merged positions frame (readPositionRows over the
signals.positions rings), so the test feeds it that wire shape: a Value
whose "positions" layer is the JSON array the desk broadcasts.
*/
const frame = (...rows: DeskPosition[]) => {
	const store = signals.positions;

	for (const key of Object.keys(store.state)) delete store.state[key];

	const value = new Value({
		origin: "positions",
		role: "update",
		scope: "all",
		index: ["positions/update/all/positions"],
		layers: [new TextEncoder().encode(JSON.stringify(rows))],
	});

	// A one-slot stand-in for the worker's ring buffer.
	store.state.all = {
		isEmpty: () => false,
		getBufferLength: () => 1,
		get: () => value,
	} as never;
};

const lot = (symbol: string, extra: Partial<DeskPosition> = {}): DeskPosition =>
	({
		id: symbol,
		symbol,
		path: "",
		virtual: false,
		opened_at: "2026-10-18T08:00:00Z",
		closed_at: null,
		qty: "1",
		entry_price: "12.345600",
		mark: "12.567800",
		...extra,
	}) as DeskPosition;

describe("Positions", () => {
	it("renders without error when there are no positions", () => {
		frame();
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("no open positions");
	});

	it("renders the open lot from the merged frame", () => {
		frame(lot("NMR/USD"));
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("NMR/USD");
		expect(markup).toContain("open");
		expect(markup).toContain("12.345600");
		expect(markup).toContain("12.567800");
	});

	it("renders closed lots in the closed section", () => {
		frame(
			lot("ABC/USD", {
				closed_at: "2026-10-18T09:00:00Z",
				profit: "3",
			}),
		);
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("closed this session");
		expect(markup).toContain("ABC/USD");
	});
});
