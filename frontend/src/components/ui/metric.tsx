import { useSelector } from "@tanstack/react-store";
import { memo, useEffect, useRef } from "react";
import { atoms } from "#/collections/app";
import {
	ensureStore,
	type Labels,
	labelsAt,
} from "#/collections/envelope";
import { terminalStore } from "#/collections/terminal";
import { sourceHeadline } from "#/components/terminal/kernel-meta";
import {
	Badge,
	Button,
	Flex,
	Meter,
	Sparkline,
	Typography,
} from "#/components/ui";
import { cn } from "#/lib/utils";
import { scaleSparklineValues, setSparkline } from "./sparkline";

const SERIES_WINDOW = 120;

/** Latest label map for this kernel at the dashboard focus symbol. */
export const useKernelReading = (kernel: string): Labels | null => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const store = ensureStore(kernel);

	return useSelector(store, (state) => labelsAt(state, focusSymbol));
};

interface MetricProps {
	name: string;
	measurement: Labels | null;
	compact?: boolean;
}

export const MetricRow = memo(
	({ name, measurement, compact = false }: MetricProps) => {
		const headline = sourceHeadline(name) ?? name;
		const rootRef = useRef<HTMLDivElement | null>(null);
		const seriesRef = useRef<number[]>([]);

		const sample =
			measurement && typeof measurement[headline] === "number"
				? measurement[headline]
				: measurement
					? (Object.values(measurement).find(
							(value) => typeof value === "number" && Number.isFinite(value),
						) ?? null)
					: null;

		useEffect(() => {
			const svg = rootRef.current?.querySelector<SVGSVGElement>(
				'[data-k="sparkline"]',
			);
			if (!svg) return;

			if (typeof sample !== "number" || !Number.isFinite(sample)) {
				return;
			}

			seriesRef.current = seriesRef.current.concat(sample).slice(-SERIES_WINDOW);
			setSparkline(svg, scaleSparklineValues(seriesRef.current), true);
		}, [sample]);

		const percent =
			typeof sample === "number" && Number.isFinite(sample)
				? Math.max(0, Math.min(100, Math.abs(sample) * 100))
				: 0;

		return (
			<div
				ref={rootRef}
				className="flex min-h-0 flex-1 flex-col overflow-hidden"
			>
				<Button
					variant="bare"
					shape="block"
					className="min-h-0 justify-start border-(--line) border-b"
					onClick={() => {
						atoms.focusMetric.set(() => name);
						terminalStore.actions.inspectSource(name);
					}}
					fullHeight
				>
					<Flex.Row
						align="center"
						justify="between"
						gap={2}
						padding={1}
						fullWidth
						className="shrink-0"
					>
						<Typography.Span
							variant="f1"
							semibold
							truncate
							className={cn(compact && "text-[10px]")}
						>
							{name.toUpperCase()}
						</Typography.Span>
						<Badge
							data-k="badge"
							data-metric-badge={headline}
							label={sample === null ? "Standby" : "Live"}
							variant={sample === null ? "disabled" : "default"}
							size="xxs"
						/>
					</Flex.Row>
					<Sparkline
						data-k="sparkline"
						data-series={headline}
						title={`${name} sparkline`}
						className="mt-0 min-h-0 flex-1"
					/>
					<Flex.Row
						align="center"
						gap={2}
						padding={1}
						fullWidth
						className="shrink-0"
					>
						<Meter
							data-metric-meter={headline}
							data-k="bar"
							layout="bar"
							size="xxs"
							percent={percent}
							variant="warning"
							className="flex-1"
							title={`${name} headline: ${headline}`}
						/>
						<Typography.Mono
							data-metric-raw={headline}
							data-format="fixed:3"
							data-k="value"
							size="xxs"
							tone="f2"
							className="w-11 shrink-0 text-right font-mono"
						>
							{sample === null || sample === undefined
								? "--"
								: sample.toFixed(3)}
						</Typography.Mono>
					</Flex.Row>
				</Button>
			</div>
		);
	},
);

MetricRow.displayName = "MetricRow";

export const Metric = ({
	name,
	compact = false,
}: {
	name: string;
	compact?: boolean;
}) => {
	const measurement = useKernelReading(name);
	return <MetricRow name={name} measurement={measurement} compact={compact} />;
};
