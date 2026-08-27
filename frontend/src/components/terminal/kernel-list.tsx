import { useSelector } from "@tanstack/react-store";
import {
	DEFAULT_KERNELS,
	focusStore,
	kernelDetailStore,
	measurementStore,
	type RingBuffer,
} from "#/collections/app";
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
import type { Measurement } from "#/providers/telemetry/telemetry/measurement";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

/*
getKernelReadings walks the focused symbol's ring for a kernel and collects the
signal-to-noise ratio per row. Rows that carry no usable metric are skipped, so
a sparse row never injects a zero into the trace.
*/
const getKernelReadings = (
	measurement: Record<string, Record<string, RingBuffer<Measurement>>>,
	kernel: string,
	symbol: string,
) => {
	const foundKernel = measurement[kernel];
	if (!foundKernel) return { points: [], latest: null };
	const foundSymbol = foundKernel[symbol];
	if (!foundSymbol) return { points: [], latest: null };

	const len = foundSymbol.getBufferLength();
	const points: number[] = [];

	for (let i = 0; i < len; i++) {
		const row = foundSymbol.get(i);
		if (!row) continue;

		const count = row.metricsLength();
		let foundValue: number | null = null;

		/*
		The kernel list plots the measurement's signal-to-noise ratio: the wire
		serializes it as a named "snr" metric, and it is the one quantity every
		source carries. Fall back to a named headline or the first metric only
		when "snr" is absent.
		*/
		for (let j = 0; j < count; j++) {
			const metric = row.metrics(j, metricObj);
			if (!metric) continue;
			if (metric.name() !== "snr") continue;
			foundValue = metric.hasNormalized() ? metric.normalized() : metric.raw();
			break;
		}

		if (foundValue === null) {
			for (let j = 0; j < count; j++) {
				const metric = row.metrics(j, metricObj);
				if (!metric) continue;
				const name = metric.name();
				if (name !== "score" && name !== "normalized") continue;
				foundValue = metric.hasNormalized() ? metric.normalized() : metric.raw();
				break;
			}
		}

		if (foundValue === null && count > 0) {
			const first = row.metrics(0, metricObj);
			if (first) {
				foundValue = first.hasNormalized() ? first.normalized() : first.raw();
			}
		}

		if (foundValue !== null && Number.isFinite(foundValue)) {
			points.push(foundValue);
		}
	}

	const latest = points.length > 0 ? points[points.length - 1] : null;
	return { points, latest };
};

/*
kernelStatus resolves a kernel row's health from what the focused symbol's ring
actually holds: a usable reading means the kernel is measuring, anything else
stays on standby until the first reading lands.
*/
const kernelStatus = (latest: number | null): SignalHealthStatus =>
	latest === null ? "waiting" : "measured";

export type KernelListProps = {
	sources?: string[];
	compact?: boolean;
};

export const KernelList = ({
	sources = DEFAULT_KERNELS,
	compact = false,
}: KernelListProps = {}) => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const measurements = useSelector(measurementStore, (state) => state);

	return (
		<div className={cn("min-h-0 overflow-auto", compact && "text-[10px]")}>
			{sources.map((source) => {
				const { points, latest } = getKernelReadings(
					measurements,
					source,
					focusSymbol,
				);
				const copy = kernelCopy(source, "");
				const status = kernelStatus(latest);
				const badge = kernelStatusMeta(status);
				const paths = kernelSparkPaths(points, status);
				const confidence =
					latest !== null && Number.isFinite(latest)
						? Math.min(1, Math.max(0, latest))
						: 0;

				return (
					<button
						key={source}
						type="button"
						data-kernel={source}
						onClick={() => kernelDetailStore.setState(() => source)}
						className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
					>
						<Flex.Row align="center" justify="between" gap={2}>
							<span className="truncate font-semibold text-(--f1)">
								{copy.name}
							</span>
							<Badge
								label={badge.label}
								variant={kernelStatusVariant(status)}
								size="xxs"
							/>
						</Flex.Row>
						<div className="mt-0.5 truncate font-mono text-[9px] text-(--f4)">
							{copy.sub}
						</div>
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
							<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line)">
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
								className="w-9 shrink-0 text-right font-mono text-[9px] tabular-nums text-(--f2)"
							>
								{latest !== null ? `${(confidence * 100).toFixed(0)}%` : "—"}
							</span>
						</Flex.Row>
					</button>
				);
			})}
		</div>
	);
};
