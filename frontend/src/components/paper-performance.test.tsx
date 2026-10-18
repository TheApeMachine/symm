// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { deskPositionsAtom } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";
import { PaperPerformance } from "./paper-performance";

const lot = (over: Partial<DeskPosition>): DeskPosition => ({
	id: over.id ?? over.symbol ?? "lot",
	symbol: over.symbol ?? "BTC/USD",
	virtual: over.virtual ?? false,
	openedAt: "2026-10-18T08:00:00Z",
	closedAt: over.closedAt ?? null,
	qty: "1",
	entryPrice: over.entryPrice ?? "10",
	mark: over.mark ?? "11",
	entryFee: null,
	exitFee: null,
	exitVwap: null,
	exitRoom: null,
	returnPct: over.returnPct ?? null,
	holdNs: 0,
});

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	deskPositionsAtom.set([]);
});

describe("PaperPerformance", () => {
	it("reads win rate and mean edge from the open lots", () => {
		deskPositionsAtom.set([
			lot({ symbol: "BTC/USD", entryPrice: "10", mark: "11" }),
			lot({ symbol: "ETH/USD", entryPrice: "10", mark: "9" }),
		]);

		const view = render(<PaperPerformance />);

		expect(view.getByText("50.0%")).toBeTruthy();
		expect(view.getByText("0.0%")).toBeTruthy();
	});

	it("prefers closed round trips once one has closed", () => {
		deskPositionsAtom.set([
			lot({ symbol: "BTC/USD", entryPrice: "10", mark: "9" }),
			lot({
				id: "done",
				symbol: "ETH/USD",
				closedAt: "2026-10-18T09:00:00Z",
				returnPct: 0.2,
			}),
		]);

		const view = render(<PaperPerformance />);

		expect(view.getByText("100.0%")).toBeTruthy();
		expect(view.getByText("20.0%")).toBeTruthy();
	});
});
