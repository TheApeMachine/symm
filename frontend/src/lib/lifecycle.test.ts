// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { positionCountAtom } from "#/collections/app";
import {
	ingestPositionFrame,
	resetLifecycles,
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
					realized: 2,
					net_return: 0.02,
					cost: 100,
					proceeds: 102,
					fees: 0.2,
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
		expect(positionCountAtom.get()).toBe(1);
		expect(report.performance).toEqual({
			closed: 1,
			wins: 1,
			win_rate: 1,
			mean_return: 0.02,
			pnl: 2,
		});
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
});
