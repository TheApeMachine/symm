import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { parseResonanceData, type ResonanceData } from "#/components/charts/prediction";

export const LiveResonanceTitle = () => {
	const symbol = useSelector(atoms.focus);
	const published = useSelector(signals.resonance, (state) =>
		state[symbol]?.end ?? 0,
	);
	const ring =
		published > 0 ? (signals.resonance.state[symbol] ?? undefined) : undefined;
	const last =
		ring && !ring.isEmpty() ? (ring.getLast() ?? undefined) : undefined;
	const artifact: ResonanceData | null = last
		? (parseResonanceData(last) ?? null)
		: null;

	const horizonVal = artifact && artifact.supportedHorizon != null ? Number(artifact.supportedHorizon) : "—";
	const reachVal = artifact?.forwardCurve ? artifact.forwardCurve.length : "—";
	const precision =
		artifact && typeof artifact.taskRelativePrecision === "number" && Number.isFinite(artifact.taskRelativePrecision)
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
