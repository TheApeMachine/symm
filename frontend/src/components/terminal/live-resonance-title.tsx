import { useSelector } from "@tanstack/react-store";
import { atoms, resonanceFrameForSymbol, signals } from "#/collections/app";
import {
	parseResonanceData,
	parseResonanceFromWorkspace,
	type ResonanceData,
} from "#/components/charts/prediction";

export const LiveResonanceTitle = () => {
	const symbol = useSelector(atoms.focus);
	const aggregate = useSelector(signals.workspace, (state) => state.aggregate);
	const last = useSelector(signals.resonance, (state) =>
		resonanceFrameForSymbol(state, symbol),
	);
	const artifact: ResonanceData | null =
		parseResonanceData(last) ??
		parseResonanceFromWorkspace(symbol, aggregate) ??
		null;

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
