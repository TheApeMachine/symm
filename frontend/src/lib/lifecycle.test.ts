// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { summarizeBook } from "#/collections/longtail";
import { signals } from "#/collections/app";
import type { DeskPosition } from "#/collections/types";
import { Value } from "#/types/value";
import {
	deskRowsForStats,
	ingestDeskClosedLots,
	ingestDeskSnapshot,
	ingestPositionFrame,
	resetLifecycles,
	selectBook,
	useLifecycles,
} from "./lifecycle";

const frame = (
	id: string,
	status: string,
	extra: Record<string, unknown> = {},
) => ({
	id,
	symbol: "BTC/USD",
	status,
	policy_epoch: "7",
	opened_at: `2026-10-10T08:0${id}:00Z`,
	closed_at: null,
	event: { at: "2026-10-10T08:00:00Z", kind: "fill", detail: status },
	mark: null,
	outcome: null,
	...extra,
});

const snapshotOf = () => renderHook(() => useLifecycles()).result.current;

afterEach(() => {
	resetLifecycles();
	vi.restoreAllMocks();
});

describe("lifecycle store", () => {
	it("folds frames into round trips, newest first, with paper performance", () => {
		ingestPositionFrame(frame("1", "holding"));
		ingestPositionFrame(frame("2", "holding"));
		ingestPositionFrame(
			frame("1", "closed", {
				closed_at: "2026-10-10T08:05:00Z",
				outcome: {
					pnl: 0.02,
					pnl_pct: 2,
					hold_ns: 1,
					expected_hold_ns: null,
					triggers: [],
				},
			}),
		);

		const { report } = snapshotOf();
		expect(report.lifecycles.map((life) => life.id)).toEqual(["2", "1"]);
		expect(report.lifecycles[1].events).toHaveLength(2);
		expect(report.open_count).toBe(1);
		expect(report.performance).toEqual({
			closed: 1,
			wins: 1,
			win_rate: 1,
			mean_return: 0.02,
			pnl: 0.02,
		});
	});

	it("installs a stored closed life in one piece", () => {
		ingestPositionFrame({
			id: "lot-1",
			symbol: "TEL/USD",
			status: "closed",
			opened_at: "2026-10-17T12:00:00Z",
			closed_at: "2026-10-17T12:10:00Z",
			events: [
				{ at: "2026-10-17T12:00:00Z", kind: "entry_match", detail: "R13/R04" },
				{ at: "2026-10-17T12:10:00Z", kind: "fill", detail: "sell fill" },
			],
			outcome: {
				pnl: 0.095,
				pnl_pct: 9.5,
				hold_ns: 600e9,
				expected_hold_ns: null,
				triggers: ["sell"],
			},
		});

		const { report } = snapshotOf();
		expect(report.lifecycles).toHaveLength(1);
		expect(report.lifecycles[0].symbol).toBe("TEL/USD");
		expect(report.lifecycles[0].events).toHaveLength(2);
		expect(report.performance.closed).toBe(1);
		expect(report.performance.pnl).toBe(0.095);
		expect(report.open_count).toBe(0);
	});

	it("keeps the latest mark without adding an event", () => {
		ingestPositionFrame(frame("1", "holding"));
		ingestPositionFrame({
			...frame("1", "holding"),
			event: null,
			mark: {
				at: "x",
				bid: 1,
				quantity: 1,
				entry_price: 1,
				unrealized: -0.5,
				unrealized_return: -0.5,
			},
		});

		const [life] = snapshotOf().report.lifecycles;
		expect(life.events).toHaveLength(1);
		expect(life.mark?.unrealized).toBe(-0.5);
	});

	it("reports and drops a malformed frame", () => {
		const error = vi.spyOn(console, "error").mockImplementation(() => {});
		ingestPositionFrame({ id: "1" });
		ingestPositionFrame({ ...frame("1", "holding"), mark: { bid: 1 } });
		expect(error).toHaveBeenCalledTimes(2);
		expect(snapshotOf().report.lifecycles).toHaveLength(0);
	});

	it("keeps a restored position whose events are null", () => {
		ingestPositionFrame({
			id: "restored",
			symbol: "ETH/USD",
			status: "holding",
			opened_at: "2026-10-17T12:00:00Z",
			events: null,
		});

		const [life] = snapshotOf().report.lifecycles;
		expect(life.symbol).toBe("ETH/USD");
		expect(life.events).toEqual([]);
		expect(snapshotOf().report.open_count).toBe(1);
	});

	it("keeps an open paper life when closed lives pass the limit", () => {
		ingestPositionFrame(
			frame("open", "holding", { opened_at: "2026-10-01T00:00:00Z" }),
		);

		for (let index = 0; index < 200; index++) {
			ingestPositionFrame({
				id: `closed-${index}`,
				symbol: "BTC/USD",
				status: "closed",
				opened_at: `2026-10-02T00:00:${String(index).padStart(2, "0")}Z`,
				events: [],
				outcome: {
					pnl: 1,
					pnl_pct: 100,
					hold_ns: 1,
					expected_hold_ns: null,
					triggers: [],
				},
			});
		}

		const ids = snapshotOf().report.lifecycles.map((life) => life.id);
		expect(ids).toContain("open");
		expect(ids).not.toContain("closed-0");
	});

	it("ingests every closed virtual row from a desk snapshot", () => {
		const rows = [
			{
				id: "a",
				symbol: "AAA/USD",
				virtual: true,
				opened_at: "2026-10-18T08:00:00Z",
				closed_at: "2026-10-18T08:01:00Z",
				pnl: 0.1,
				pnl_pct: 10,
			},
			{
				id: "b",
				symbol: "BBB/USD",
				virtual: true,
				opened_at: "2026-10-18T09:00:00Z",
				closed_at: "2026-10-18T09:01:00Z",
				pnl: -0.1,
				pnl_pct: -10,
			},
		];
		ingestDeskClosedLots(rows);
		const closed = snapshotOf()
			.report.lifecycles.filter((life) => life.status === "closed")
			.map((life) => life.id);
		expect(closed.sort()).toEqual(["a", "b"]);
	});

	it("keeps closed desk lots in deskRowsForStats after they leave the hub snapshot", () => {
		const row = {
			id: "closed-up",
			symbol: "UP/USD",
			virtual: true,
			opened_at: "2026-10-18T08:00:00Z",
			closed_at: "2026-10-18T09:00:00Z",
			pnl: 0.002,
			pnl_pct: 0.2,
		} as DeskPosition;
		const value = new Value({
			origin: "positions",
			role: "update",
			scope: "UP-USD",
		});
		value.layers = [new TextEncoder().encode(JSON.stringify(row))];
		value.index = [`${value.prefix}/position`];
		signals.positions.setState(() => ({ "UP-USD": value }));
		ingestDeskSnapshot();
		signals.positions.setState(() => ({}));

		const stats = summarizeBook(deskRowsForStats(), "virtual");
		expect(stats.closed).toBe(1);
		expect(stats.meanReturn).toBeCloseTo(0.002);
	});

	it("keeps a virtual life out of the paper book", () => {
		ingestPositionFrame({
			id: "R01/R05",
			symbol: "TEL/USD",
			status: "open",
			opened_at: "2026-10-17T12:00:00Z",
			events: [],
			book: "virtual",
		});
		ingestPositionFrame(frame("1", "holding"));

		const report = snapshotOf().report;
		expect(selectBook(report, "paper").lifecycles).toHaveLength(1);
		expect(selectBook(report, "virtual").lifecycles).toHaveLength(1);
		expect(selectBook(report, "virtual").open_count).toBe(1);
	});
});
