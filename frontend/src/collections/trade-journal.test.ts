import { describe, expect, it } from "vitest";
import type { TradeObservation } from "#/types/thesis";
import { Circular } from "./circular";
import {
	pushJournalObservations,
	tradeJournalStore,
	tradeJournalValues,
} from "./trade-journal";

const observation = (
	overrides: Partial<TradeObservation>,
): TradeObservation => ({
	kind: "execution",
	symbol: "BTC/USD",
	decision: 0,
	at: "2026-07-14T12:00:00Z",
	...overrides,
});

describe("pushJournalObservations", () => {
	it("appends only unseen observations from cumulative snapshots", () => {
		const journal = Circular<TradeObservation>(4);
		const firstPass = pushJournalObservations(journal, [
			observation({ at: "2026-07-14T12:00:00Z", status: "partially_filled" }),
			observation({ at: "2026-07-14T12:00:01Z", status: "filled" }),
		]);

		expect(firstPass.appended).toBe(true);
		const secondPass = pushJournalObservations(firstPass.journal, [
			observation({ at: "2026-07-14T12:00:00Z", status: "partially_filled" }),
			observation({ at: "2026-07-14T12:00:01Z", status: "filled" }),
			observation({
				kind: "lifecycle_transition",
				at: "2026-07-14T12:00:02Z",
				status: "closed",
			}),
		]);

		expect(secondPass.appended).toBe(true);
		expect(
			tradeJournalValues(secondPass.journal).map((row) => row.status),
		).toEqual(["partially_filled", "filled", "closed"]);
	});

	it("returns a new journal reference when rows are appended", () => {
		const journal = Circular<TradeObservation>(4);
		const result = pushJournalObservations(journal, [
			observation({ at: "2026-07-14T12:00:00Z", status: "entered" }),
		]);

		expect(result.appended).toBe(true);
		expect(result.journal).not.toBe(journal);
	});

	it("evicts the oldest rows once the circular buffer is full", () => {
		const journal = Circular<TradeObservation>(2);
		const firstPass = pushJournalObservations(journal, [
			observation({ at: "2026-07-14T12:00:00Z", status: "a" }),
			observation({ at: "2026-07-14T12:00:01Z", status: "b" }),
		]);
		const secondPass = pushJournalObservations(firstPass.journal, [
			observation({ at: "2026-07-14T12:00:02Z", status: "c" }),
		]);

		expect(
			tradeJournalValues(secondPass.journal).map((row) => row.status),
		).toEqual(["b", "c"]);
	});
});

describe("tradeJournalStore", () => {
	it("retains appended journal history across shorter replay frames", () => {
		tradeJournalStore.actions.reset();
		tradeJournalStore.actions.updateFrame([
			observation({ at: "2026-07-14T12:00:00Z", status: "entered" }),
			observation({ at: "2026-07-14T12:00:01Z", status: "filled" }),
		]);
		tradeJournalStore.actions.updateFrame([
			observation({ at: "2026-07-14T12:00:01Z", status: "filled" }),
		]);

		expect(
			tradeJournalValues(tradeJournalStore.state.journal).map(
				(row) => row.status,
			),
		).toEqual(["entered", "filled"]);
	});

	it("marks the store observed on empty frames", () => {
		tradeJournalStore.actions.reset();
		expect(tradeJournalStore.state.observed).toBe(false);

		tradeJournalStore.actions.updateFrame([]);

		expect(tradeJournalStore.state.observed).toBe(true);
	});
});
