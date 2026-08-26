import { useSelector } from "@tanstack/react-store";
import {
	DEFAULT_KERNELS,
	focusStore,
	kernelDetailStore,
	measurementStore,
	type RingBuffer,
} from "#/collections/app";
import { Flex } from "#/components/ui";
import { cn } from "#/lib/utils";
import type { Measurement } from "#/providers/telemetry/telemetry/measurement";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

const computeSparkline = (
	points: number[],
	width = 60,
	height = 14,
	padding = 1,
): string => {
	if (points.length < 2) return "";

	let min = points[0];
	let max = points[0];
	for (let i = 1; i < points.length; i++) {
		if (points[i] < min) min = points[i];
		if (points[i] > max) max = points[i];
	}

	const range = max - min || 1;
	const count = points.length;
	let d = "";

	for (let i = 0; i < count; i++) {
		const x = (i / (count - 1)) * width;
		const y =
			height - padding - ((points[i] - min) / range) * (height - padding * 2);
		d += `${(i === 0 ? "M " : " L ") + x.toFixed(1)},${y.toFixed(1)}`;
	}

	return d;
};

const getKernelMetrics = (
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

		for (let j = 0; j < count; j++) {
			const metric = row.metrics(j, metricObj);
			if (!metric) continue;
			const name = metric.name();
			if (name === "snr" || name === "score" || name === "normalized") {
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
				const { points, latest } = getKernelMetrics(
					measurements,
					source,
					focusSymbol,
				);
				const sparklinePath = computeSparkline(points);

				return (
					<button
						key={source}
						type="button"
						data-kernel={source}
						onClick={() => kernelDetailStore.setState(() => source)}
						className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
					>
						<Flex.Row align="center" justify="between" gap={2}>
							<span className={cn("truncate font-semibold text-(--f1)")}>
								{source}
							</span>
							<span
								data-k="symbol"
								className="font-mono text-[9.5px] text-(--f4)"
							>
								{latest !== null ? focusSymbol : ""}
							</span>
						</Flex.Row>
						<Flex.Row
							align="center"
							justify="between"
							gap={2}
							className="mt-0.5 font-mono text-[9.5px] text-(--f4)"
						>
							<span data-k="snr1" className="text-(--acc)">
								{latest !== null ? latest.toFixed(1) : "—"}
							</span>
							<svg
								viewBox="0 0 60 14"
								className="h-3.5 w-15 shrink-0 overflow-visible"
								preserveAspectRatio="none"
							>
								<title>sparkline</title>
								<path
									data-k="sparkline"
									d={sparklinePath}
									fill="none"
									stroke="var(--acc)"
									strokeWidth="1.5"
									strokeLinecap="round"
									strokeLinejoin="round"
									vectorEffect="non-scaling-stroke"
								/>
							</svg>
						</Flex.Row>
					</button>
				);
			})}
		</div>
	);
};
