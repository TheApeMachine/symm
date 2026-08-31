import { useSelector } from "@tanstack/react-store";
import { focusStore, resonanceArtifactStore } from "#/collections/app";

export const LiveResonanceTitle = () => {
	const symbol = useSelector(focusStore, (state) => state);
	const artifact = useSelector(resonanceArtifactStore, (state) =>
		state.findLast((row) => row.symbol() === symbol),
	);

	// Reach is how far the forward curve actually extends, which is the probe
	// horizon the coder reported: the curve is one element per horizon step.
	const horizon = artifact ? String(artifact.supportedHorizon()) : "—";
	const reach = artifact ? String(artifact.forwardCurveLength()) : "—";
	const precision = artifact
		? artifact.taskRelativePrecision().toFixed(3)
		: "—";

	return (
		<span>
			h<span data-res="horizon">{horizon}</span>
			{" · r "}
			<span data-res="reach">{reach}</span>
			{" · relative precision "}
			<span data-res="precision">{precision}</span>
		</span>
	);
};
