// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";
import { Value } from "#/types/value";
import { OpenPositions } from "./open-positions";

const lotFrame = (row: DeskPosition) => {
	const value = new Value({
		origin: "positions",
		role: "update",
		scope: String(row.symbol).replace(/\//g, "-"),
	});

	value.layers = [new TextEncoder().encode(JSON.stringify(row))];
	value.index = [`${value.prefix}/position`];

	return value;
};

const ingest = (...rows: DeskPosition[]) => {
	signals.positions.setState(() =>
		Object.fromEntries(rows.map((row) => [lotFrame(row).scope, lotFrame(row)])),
	);
};

const lot = (over: Partial<DeskPosition>): DeskPosition =>
	({
		id: over.id ?? over.symbol ?? "lot",
		symbol: over.symbol ?? "RLC/USD",
		virtual: over.virtual ?? false,
		opened_at: over.opened_at ?? "2026-10-18T08:00:00Z",
		closed_at: over.closed_at ?? null,
		qty: over.qty ?? "2",
		entry_price: over.entry_price ?? "1",
		mark: over.mark ?? "1.05",
		entry_fee: over.entry_fee ?? "0.01",
		exit_fee: over.exit_fee ?? null,
		exit_vwap: over.exit_vwap ?? null,
		exit_room: over.exit_room ?? null,
		book_return: over.book_return ?? 5,
		hold_ns: over.hold_ns ?? 75_000_000_000,
	}) as DeskPosition;

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	signals.positions.setState(() => ({}));
});

describe("OpenPositions", () => {
	it("shows the empty state when nothing is held", () => {
		ingest();
		const view = render(<OpenPositions />);
		expect(view.getByText("No open positions")).toBeTruthy();
	});

	it("draws a card from the position JSON the desk sent", () => {
		ingest(lot({ symbol: "RLC/USD" }));
		const view = render(<OpenPositions />);
		expect(view.getByText("RLC/USD")).toBeTruthy();
		expect(view.getByText("held 1m 15s")).toBeTruthy();
		expect(view.getByText("5.0%")).toBeTruthy();
		expect(view.getByText("qty 2")).toBeTruthy();
	});

	it("shows exit room on the gauge when the book covers the lot", () => {
		ingest(lot({ symbol: "RLC/USD", exit_room: 12.5 }));
		const view = render(<OpenPositions />);
		expect(view.getByText("12.5×")).toBeTruthy();
	});

	it("keeps a virtual lot off the paper book", () => {
		ingest(
			lot({ symbol: "BTC/USD" }),
			lot({ id: "trial", symbol: "ETH/USD", virtual: true }),
		);
		const paper = render(<OpenPositions />);
		expect(paper.getByText("BTC/USD")).toBeTruthy();
		expect(paper.queryByText("ETH/USD")).toBeNull();
		paper.unmount();

		bookAtom.set("virtual");
		const virtual = render(<OpenPositions />);
		expect(virtual.getByText("ETH/USD")).toBeTruthy();
		expect(virtual.queryByText("BTC/USD")).toBeNull();
	});
});
