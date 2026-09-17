import { useSelector } from "@tanstack/react-store";
import { focusStore, resonanceStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";

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
		<Typography.Span variant="f3">
			h
			<Typography.Span data-res="horizon" variant="f1">
				{String(horizonVal)}
			</Typography.Span>
			{" · r "}
			<Typography.Span data-res="reach" variant="f1">
				{String(reachVal)}
			</Typography.Span>
			{" · relative precision "}
			<Typography.Span data-res="precision" variant="f1">
				{precision}
			</Typography.Span>
		</Typography.Span>
	);
};
