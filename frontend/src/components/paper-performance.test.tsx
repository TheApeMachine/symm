// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";
import {
	ingestDeskSnapshot,
	resetLifecycles,
} from "#/lib/lifecycle";
import { Value } from "#/types/value";
import { PaperPerformance } from "./paper-performance";

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
		symbol: over.symbol ?? "BTC/USD",
		virtual: over.virtual ?? false,
		opened_at: over.opened_at ?? "2026-10-18T08:00:00Z",
		closed_at: over.closed_at ?? null,
		qty: over.qty ?? "1",
		entry_price: over.entry_price ?? "10",
		mark: over.mark ?? "11",
		book_return: over.book_return ?? null,
		net_return: over.net_return ?? null,
		cost: over.cost ?? null,
		proceeds: over.proceeds ?? null,
		profit: over.profit ?? null,
	}) as DeskPosition;

afterEach(() => {
	cleanup();
	resetLifecycles();
	bookAtom.set("paper");
	signals.positions.setState(() => ({}));
});

describe("PaperPerformance", () => {
	it("shows dashes until a priced close is recorded in the lifecycle store", () => {
		ingest(lot({ symbol: "BTC/USD", book_return: 10, virtual: true }));
		const view = render(<PaperPerformance />);
		expect(view.getAllByText("—").length).toBeGreaterThanOrEqual(2);
	});

	it("reads win rate and edge from settled virtual closes the session remembers", () => {
		ingest(
			lot({
				id: "done",
				symbol: "UP/USD",
				virtual: true,
				closed_at: "2026-10-18T09:00:00Z",
				cost: "10",
				proceeds: "12",
				profit: "2",
				net_return: 20,
			}),
		);
		ingestDeskSnapshot();
		bookAtom.set("virtual");

		const view = render(<PaperPerformance />);

		expect(view.getByText("100.0%")).toBeTruthy();
		expect(view.getByText("20.0%")).toBeTruthy();
	});
});
