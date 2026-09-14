import { useSelector } from "@tanstack/react-store";
import { focusStore, resonanceStore } from "#/collections/app";

export const LiveResonanceTitle = () => {
	const symbol = useSelector(focusStore, (state) => state);
	const artifact = useSelector(resonanceStore, (state) => {
		const ring = state[symbol];
		return ring && !ring.isEmpty() ? (ring.getLast() as any) : null;
	});

	const horizonVal = artifact
		? typeof artifact.supportedHorizon === "function"
			? artifact.supportedHorizon()
			: (artifact.supportedHorizon ?? "—")
		: "—";

	const reachVal = artifact
		? typeof artifact.forwardCurveLength === "function"
			? artifact.forwardCurveLength()
			: Array.isArray(artifact.forwardCurve)
				? artifact.forwardCurve.length
				: "—"
		: "—";

	const precisionNum = artifact
		? typeof artifact.taskRelativePrecision === "function"
			? artifact.taskRelativePrecision()
			: typeof artifact.taskRelativePrecision === "number"
				? artifact.taskRelativePrecision
				: null
		: null;

	const precision =
		precisionNum !== null && Number.isFinite(precisionNum)
			? precisionNum.toFixed(3)
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
