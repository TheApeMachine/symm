import { batch } from "@tanstack/store";
import { atoms, signals } from "#/collections/app";
import { bookAtom, decisionBook } from "#/collections/book";
import { cognitionStore } from "#/collections/cognition";
import type { Decision } from "#/collections/envelope";

/*
syncEngineTelemetry keeps the dashboard engine readout aligned with the
live decision and market tick streams for the selected ledger.
*/
export const syncEngineTelemetry = () => {
	const { matches, tree } = cognitionStore.state;
	const live = signals.decisions.state as Record<string, Decision>;
	const book = bookAtom.get();

	let candidates = 0;
	for (const match of Object.values(matches)) {
		if (match.action && match.action !== "") {
			candidates++;
		}
	}
	for (const decision of Object.values(live)) {
		if (!decision?.action || decision.action === "skip") continue;
		if (decisionBook(decision) !== book) continue;
		candidates++;
	}

	let phase = atoms.phase.get();
	if (tree?.trading) {
		phase = "trading";
	} else if (tree?.gate?.enabled) {
		phase = "evaluate";
	} else if (atoms.tickCount.get() > 0) {
		phase = "live";
	}

	batch(() => {
		if (atoms.candidates.get() !== candidates) {
			atoms.candidates.set(candidates);
		}
		if (atoms.phase.get() !== phase) {
			atoms.phase.set(phase);
		}
	});
};
