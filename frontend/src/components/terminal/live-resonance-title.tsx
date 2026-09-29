import { useSelector } from "@tanstack/react-store";
import { focusAtom, resonanceStore } from "#/collections/app";
import type { ResonanceT } from "#/providers/telemetry/telemetry/resonance";

export const LiveResonanceTitle = () => {
	const symbol = useSelector(focusAtom);
	const artifact: ResonanceT | null = useSelector(resonanceStore, (state) => {
		const ring = state[symbol];
		return ring && !ring.isEmpty() ? (ring.getLast() ?? null) : null;
	});

	const horizonVal = artifact ? Number(artifact.supportedHorizon) : "—";
	const reachVal = artifact ? artifact.forwardCurve.length : "—";
	const precision =
		artifact && Number.isFinite(artifact.taskRelativePrecision)
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
