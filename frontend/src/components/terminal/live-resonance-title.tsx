import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import type { Resonance } from "#/collections/envelope";
import {
	parseResonanceData,
	type ResonanceData,
} from "#/components/charts/prediction";

const resonanceAt = (
	state: Record<string, Resonance>,
	symbol: string,
): Resonance | undefined =>
	state[symbol] ?? state[symbol.replace(/\//g, "-")];

export const LiveResonanceTitle = () => {
	const symbol = useSelector(atoms.focus);
	const last = useSelector(signals.resonance, (state) =>
		resonanceAt(state, symbol),
	);
	const artifact: ResonanceData | null =
		parseResonanceData(last as unknown as Record<string, unknown>) ?? null;

	const horizonVal =
		artifact && artifact.supportedHorizon != null
			? Number(artifact.supportedHorizon)
			: "—";
	const reachVal = artifact?.forwardCurve ? artifact.forwardCurve.length : "—";
	const precision =
		artifact &&
		typeof artifact.taskRelativePrecision === "number" &&
		Number.isFinite(artifact.taskRelativePrecision)
			? artifact.taskRelativePrecision.toFixed(3)
			: "—";

	return (
		<span>
			h<span data-res="horizon">{String(horizonVal)}</span>
			{" · r "}
			<span data-res="reach">{String(reachVal)}</span>
			{" · relative precision "}
			<span data-res="precision">{precision}</span>
		</span>
	);
};
