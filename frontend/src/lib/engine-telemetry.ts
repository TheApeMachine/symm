import { atoms } from "#/collections/app";
import { cognitionStore } from "#/collections/cognition";

/*
syncEngineTelemetry keeps the dashboard engine readout aligned with the
live cognition and market tick streams.
*/
export const syncEngineTelemetry = () => {
	const { matches, tree } = cognitionStore.state;

	let candidates = 0;
	for (const match of Object.values(matches)) {
		if (match.action && match.action !== "") {
			candidates++;
		}
	}
	atoms.candidates.set(candidates);

	if (tree?.trading) {
		atoms.phase.set("trading");
		return;
	}

	if (tree?.gate?.enabled) {
		atoms.phase.set("evaluate");
		return;
	}

	if (atoms.tickCount.get() > 0) {
		atoms.phase.set("live");
	}
};
