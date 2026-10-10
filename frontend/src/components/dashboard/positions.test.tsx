import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { closedPositionsAtom, positionsAtom } from "#/collections/app";
import { HoldingT } from "#/providers/telemetry/telemetry/holding";
import { PositionT } from "#/providers/telemetry/telemetry/position";
import { SellEventT } from "#/providers/telemetry/telemetry/sell-event";
import { Positions } from "./positions";

describe("Positions", () => {
	it("renders without error when positionsAtom is empty", () => {
		positionsAtom.set([]);
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("no open positions");
	});

	it("renders active open positions when positionsAtom contains positions", () => {
		const original = positionsAtom.get();
		try {
			const pos = new PositionT();
			pos.status = "active";
			const holding = new HoldingT();
			holding.symbol = "NMR/USD";
			holding.status = "active";
			holding.pnl = "0.4200";
			holding.entryPrice = "12.345600";
			holding.mark = "12.567800";
			holding.returnPct = 3.41;
			pos.holding = holding;

			positionsAtom.set([pos]);
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("NMR/USD");
			expect(markup).toContain("active");
			expect(markup).toContain("0.4200 USD");
			expect(markup).not.toContain("EXIT");
			expect(markup).toContain("12.345600");
			expect(markup).toContain("12.567800");
			expect(markup).not.toContain("no open positions");
		} finally {
			positionsAtom.set(original);
		}
	});

	it("shows exit capacity, budget, venue beside shadow P&L, and sell trigger markers", () => {
		const original = positionsAtom.get();
		const originalClosed = closedPositionsAtom.get();
		try {
			const pos = new PositionT();
			const holding = new HoldingT();
			holding.symbol = "NMR/USD";
			holding.status = "holding";
			holding.pnl = "0.1";
			holding.capacityDefined = true;
			holding.capacityRatio = 0.5;
			holding.slippageBudget = 0.017;
			holding.budgetSource = "matched_edge";
			holding.venuePnl = "1.25";
			holding.shadowPnl = "-0.75";
			holding.shadowDefined = true;
			const trim = new SellEventT();
			trim.trigger = "capacity_trim";
			trim.qty = "2.5";
			trim.capacityQty = 2.5;
			trim.capacityRatio = 0.5;
			holding.sells = [trim];
			pos.holding = holding;

			const closed = new HoldingT();
			closed.symbol = "ABC/USD";
			closed.status = "closed";
			closed.venuePnl = "3";
			closed.shadowDefined = false;
			closed.closedAt = 1n;
			const exit = new SellEventT();
			exit.trigger = "learned_exit";
			exit.qty = "1";
			closed.sells = [exit];

			positionsAtom.set([pos]);
			closedPositionsAtom.set([closed]);
			const markup = renderToStaticMarkup(<Positions />);

			expect(markup).toContain("capacity 0.50×");
			expect(markup).toContain("budget 1.700% matched_edge");
			expect(markup).toContain("venue 1.2500");
			expect(markup).toContain("shadow -0.7500");
			expect(markup).toContain('data-trigger="capacity_trim"');
			expect(markup).toContain("TRIM 2.5000");
			expect(markup).toContain("closed this session");
			expect(markup).toContain('data-trigger="learned_exit"');
			expect(markup).toContain("shadow —");
		} finally {
			positionsAtom.set(original);
			closedPositionsAtom.set(originalClosed);
		}
	});
});
