import { useSelector } from "@tanstack/react-store";
import {
	DEFAULT_KERNELS,
	focusStore,
	getMeasurementStore,
	kernelDetailStore,
	resonanceArtifactStore,
} from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	kernelStatusVariant,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import { Flex } from "#/components/ui";
import { Badge } from "#/components/ui/badge";
import { cn } from "#/lib/utils";
import type { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import type { EnvelopeResonanceArtifact } from "#/providers/telemetry/telemetry/envelope-resonance-artifact";
import type { FrameBuffer } from "#/collections/app";

/*
getKernelReadings walks a kernel's own ring (already scoped to the focused
symbol server-side — the backend never ships a measurement for any other
symbol) and collects each row's signal-to-noise ratio. SNR is a top-level
field on the wire measurement (data.Measurement.SNR/SNRDefined on the Go
side), not an entry in its metrics map — Finalize() only sets SNRDefined
once a real noise model or covariance was estimable, so a row with
snrDefined() false has no SNR reading yet and is skipped rather than
plotting some other metric in its place.
*/
const getKernelReadings = (ring: FrameBuffer<EnvelopeMeasurement>) => {
	const len = ring.getBufferLength();
	const points: number[] = [];

	for (let i = 0; i < len; i++) {
		const row = ring.get(i);
		if (!row) continue;
		if (!row.snrDefined()) continue;

		const snr = row.snr();
		if (Number.isFinite(snr)) points.push(snr);
	}

	const latest = points.length > 0 ? points[points.length - 1] : null;
	return { points, latest };
};

/*
getResonanceReadings walks the shared resonanceArtifactStore ring for the
frames that belong to the focused symbol — unlike every other kernel's ring, resonance is
not pre-scoped to one symbol server-side (logic/resonance.Solver.Update keys
its predictive coder per symbol across the whole cross-section), so the
symbol filter has to happen here, the same way live-resonance-title.tsx and
xray.tsx already do. Confidence is the collected reading: unlike SNR it is
already a real [0,1] quantity by construction (learning.PredictiveOutput's
own doc comment), but it is only meaningful once the predictive head has
resolved enough outcomes to calibrate — calibrated() is that honest gate, the
same role snrDefined() plays for the other kernels, so an uncalibrated frame
is skipped rather than plotting a confidence value that isn't real yet.
*/
const getResonanceReadings = (
	ring: FrameBuffer<EnvelopeResonanceArtifact>,
	focusSymbol: string,
) => {
	const len = ring.getBufferLength();
	const points: number[] = [];

	for (let i = 0; i < len; i++) {
		const row = ring.get(i);
		if (!row) continue;
		if (row.symbol() !== focusSymbol) continue;
		if (!row.calibrated()) continue;

		const confidence = row.confidence();
		if (Number.isFinite(confidence)) points.push(confidence);
	}

	const latest = points.length > 0 ? points[points.length - 1] : null;
	return { points, latest };
};

/*
kernelStatus resolves a kernel row's health from what its own ring actually
holds: a usable reading means the kernel is measuring, anything else stays
on standby until the first reading lands.
*/
const kernelStatus = (latest: number | null): SignalHealthStatus =>
	latest === null ? "waiting" : "measured";

/*
relativeToOwnRange scales each SNR reading against the min/max this kernel's
own ring has actually observed. SNR is an unbounded Mahalanobis/scalar
quantity (divergence²/noise_variance) with no fixed "good" threshold declared
anywhere in the backend, so there is no principled absolute [0,1] mapping to
assert — asserting one (e.g. snr/(snr+k)) would invent a quality bar that
doesn't exist in the domain. Scaling relative to the kernel's own recent
range instead shows genuine relative movement without claiming any reading
is universally strong or weak. A single reading (no range yet) reads as its
own peak, at the top of the trace, since nothing else exists yet to compare
it against.
*/
const relativeToOwnRange = (values: number[]): number[] => {
	if (values.length === 0) return [];

	const min = Math.min(...values);
	const max = Math.max(...values);
	const range = max - min;

	return values.map((value) => (range > 0 ? (value - min) / range : 1));
};

/*
Resonance is not an EnvelopeMeasurement (no SNR/metrics map — see
getResonanceReadings) and its ring is not pre-scoped to the focused symbol,
so it reads from resonanceArtifactStore directly instead of
getMeasurementStore. Both selectors below run unconditionally (hooks cannot
be conditional); only one of their results is actually used per row.
*/
const isResonance = (source: string) => source === "resonance";

const KernelRow = ({
	source,
	compact,
}: {
	source: string;
	compact: boolean;
}) => {
	const resonance = isResonance(source);
	const measurementRing = useSelector(getMeasurementStore(source), (state) => state);
	const resonanceRing = useSelector(resonanceArtifactStore, (state) => state);
	const focusSymbol = useSelector(focusStore, (state) => state);

	const { points, latest } = resonance
		? getResonanceReadings(resonanceRing, focusSymbol)
		: getKernelReadings(measurementRing);
	const copy = kernelCopy(source, "");
	const status = kernelStatus(latest);
	const badge = kernelStatusMeta(status);

	// Confidence is already a real [0,1] quantity by construction (see
	// getResonanceReadings) — only unbounded SNR needs scaling against its
	// own observed range before it means anything as a bar/sparkline.
	const relativePoints = resonance ? points : relativeToOwnRange(points);
	const paths = kernelSparkPaths(relativePoints, status);
	const confidence = relativePoints.length > 0 ? relativePoints[relativePoints.length - 1] : 0;
	const barTitle = resonance
		? "Predictive confidence for the focused symbol, once the head has calibrated"
		: "SNR relative to this kernel's own recent range — not an absolute quality threshold";
	const valueLabel = resonance ? "confidence" : "raw SNR";
	const valueText = latest === null ? "—" : resonance ? `${(latest * 100).toFixed(0)}%` : latest.toFixed(2);

	return (
		<button
			type="button"
			data-kernel={source}
			onClick={() => {
				kernelDetailStore.setState(() => source);
				terminalStore.actions.inspectSource(source);
			}}
			className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
		>
			<Flex.Row align="center" justify="between" gap={2}>
				<span className={cn("truncate font-semibold text-(--f1)", compact && "text-[10px]")}>
					{copy.name}
				</span>
				<Badge label={badge.label} variant={kernelStatusVariant(status)} size="xxs" />
			</Flex.Row>
			<div className="mt-0.5 truncate font-mono text-[9px] text-(--f4)">{copy.sub}</div>
			<svg
				viewBox="0 0 150 30"
				preserveAspectRatio="none"
				className="mt-1.5 block h-6.5 w-full"
			>
				<title>{`${copy.name} sparkline`}</title>
				<polyline points={paths.area} fill={paths.fill} stroke="none" />
				<polyline
					points={paths.spark}
					fill="none"
					stroke={paths.line}
					strokeWidth="1.4"
					vectorEffect="non-scaling-stroke"
				/>
			</svg>
			<Flex.Row align="center" gap={2} className="mt-1.5">
				<div
					className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line)"
					title={barTitle}
				>
					<div
						data-k="conf"
						className="h-full transition-[width,background-color] duration-300 ease-out"
						style={{
							width: `${(confidence * 100).toFixed(1)}%`,
							background: paths.line,
						}}
					/>
				</div>
				<span
					data-k="snr1"
					className="w-11 shrink-0 text-right font-mono text-[9px] tabular-nums text-(--f2)"
					title={valueLabel}
				>
					{valueText}
				</span>
			</Flex.Row>
		</button>
	);
};

export type KernelListProps = {
	sources?: string[];
	compact?: boolean;
};

export const KernelList = ({
	sources = DEFAULT_KERNELS,
	compact = false,
}: KernelListProps = {}) => {
	return (
		<div className={cn("min-h-0 overflow-auto", compact && "text-[10px]")}>
			{sources.map((source) => (
				<KernelRow key={source} source={source} compact={compact} />
			))}
		</div>
	);
};
