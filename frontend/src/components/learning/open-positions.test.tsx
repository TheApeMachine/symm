// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";
import { Value } from "#/types/value";
import { OpenPositions, positionRows } from "./open-positions";

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
		pnl: over.pnl ?? 0.05,
		pnl_pct: over.pnl_pct ?? 5,
		hold_ns: over.hold_ns ?? 75_000_000_000,
		...over,
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
		expect(view.getByText(/ahead after fees|behind after fees|roughly flat/i)).toBeTruthy();
	});

	it("shows exit room on the gauge when the book covers the lot", () => {
		ingest(lot({ symbol: "RLC/USD", exit_room: 12.5 }));
		const view = render(<OpenPositions />);
		expect(view.getByText("12.5×")).toBeTruthy();
	});

	it("draws profit and stop marks on the virtual P&L gauge", () => {
		ingest(
			lot({
				symbol: "AKE/USD",
				virtual: true,
				pnl: -0.23,
				pnl_pct: -2.3,
				open_pnl: -0.115,
			}),
		);
		bookAtom.set("virtual");
		const view = render(<OpenPositions />);
		const gauges = view.container.querySelectorAll('[role="img"]');
		const pnlGauge = gauges[0];
		expect(pnlGauge?.querySelectorAll("line").length).toBe(2);
		expect(pnlGauge?.innerHTML).toContain("Profit exit");
		expect(pnlGauge?.innerHTML).toContain("Stop exit");
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

describe("position rows", () => {
	afterEach(() => {
		cleanup();
		signals.positions.setState(() => ({}));
	});

	it("keeps the newer mark and the lots the update did not mention", () => {
		const snap = new Value({
			origin: "positions",
			role: "update",
			scope: "all",
			tick: 1n,
		});
		const held = [
			lot({ id: "bmt", symbol: "BMT/USD", mark: "1" }),
			lot({ id: "eth", symbol: "ETH/USD", mark: "3" }),
		];
		snap.layers = [new TextEncoder().encode(JSON.stringify(held))];
		snap.index = [`${snap.prefix}/positions`];

		const moved = lotFrame(lot({ id: "bmt", symbol: "BMT/USD", mark: "2" }));
		moved.tick = 2n;

		signals.positions.setState(() => ({ all: snap, [moved.scope]: moved }));

		const rows = positionRows(signals.positions.state);
		const bmt = rows.find((row) => row.id === "bmt");
		const eth = rows.find((row) => row.id === "eth");

		expect(rows).toHaveLength(2);
		expect(bmt?.mark).toBe("2");
		expect(eth?.mark).toBe("3");
	});
});
