import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
import { Typography } from "#/components/ui/typography";

const readMetric = (obj: any, key: string): number | null => {
	if (!obj) return null;
	if (typeof obj[key] === "function") {
		const val = obj[key]();
		return typeof val === "number" ? val : null;
	}
	if (typeof obj[key] === "number") return obj[key];
	if (Array.isArray(obj.metrics)) {
		const metric = obj.metrics.find((m: any) => m?.name === key);
		if (metric && typeof metric.raw === "number") return metric.raw;
	}
	return null;
};

export const LiveResonanceTitle = () => {
	const symbol = useSelector(focusAtom, (state) => state);
	const artifact = useSelector(signals.resonance, (state) => {
		const ring = state[symbol];
		return ring && !ring.isEmpty() ? (ring.getLast() as any) : null;
	});

	const horizonMetric = readMetric(artifact, "supportedHorizon");
	const horizonVal =
		horizonMetric !== null
			? horizonMetric
			: artifact
				? typeof artifact.supportedHorizon === "function"
					? artifact.supportedHorizon()
					: (artifact.supportedHorizon ?? "—")
				: "—";

	const reachMetric = readMetric(artifact, "forwardCurveLength");
	const reachVal =
		reachMetric !== null
			? reachMetric
			: artifact
				? typeof artifact.forwardCurveLength === "function"
					? artifact.forwardCurveLength()
					: Array.isArray(artifact.forwardCurve)
						? artifact.forwardCurve.length
						: "—"
				: "—";

	const precisionNum = readMetric(artifact, "taskRelativePrecision");

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
