import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Scanlines } from "#/components/ui/scanlines";
import { cn } from "#/lib/utils";

export type ChartSeries = {
	values: Array<number | null | undefined>;
	stroke: string;
	fill?: string;
	label?: string;
};

export type MetricChartProps = {
	className?: string;
	/** Parallel sample timestamps (ms) for the x-axis */
	times: number[];
	series: ChartSeries[];
	formatY: (value: number) => string;
	/** When set, the y-axis uses this domain instead of the data's min/max */
	yDomain?: [number, number];
	baseline?: number;
};

const MARGIN = { top: 8, right: 10, bottom: 20, left: 42 };

const formatTime = (ms: number) =>
	new Date(ms).toLocaleTimeString(undefined, {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
	});

const niceTicks = (min: number, max: number, count = 4): number[] => {
	if (!Number.isFinite(min) || !Number.isFinite(max)) {
		return [];
	}

	if (min === max) {
		return [min];
	}

	const step = (max - min) / Math.max(count - 1, 1);
	return Array.from({ length: count }, (_, i) => min + step * i);
};

type Point = { x: number; y: number; value: number };

const buildSegments = (
	values: Array<number | null | undefined>,
	toPoint: (index: number, value: number) => Point,
): Point[][] => {
	const segments: Point[][] = [];
	let current: Point[] = [];

	for (let index = 0; index < values.length; index++) {
		const value = values[index];
		if (value === null || value === undefined || !Number.isFinite(value)) {
			if (current.length > 0) {
				segments.push(current);
				current = [];
			}
			continue;
		}

		current.push(toPoint(index, value));
	}

	if (current.length > 0) {
		segments.push(current);
	}

	return segments;
};

/*
MetricChart draws one or more series in the container's actual pixel size:
a ResizeObserver measures the cell and the svg is drawn at that size, so the
axes, grid and line are never stretched to fit a viewBox. The shared
SeriesChart scales a fixed 400x100 viewBox to whatever box it is given, which
is how the old long-tail panels ended up twice as tall as they were wide.
*/
export const MetricChart = ({
	className,
	times,
	series,
	formatY,
	yDomain,
	baseline,
}: MetricChartProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const [size, setSize] = useState({ width: 0, height: 0 });
	const patternId = useId();

	useEffect(() => {
		const element = ref.current;
		if (!element) return;

		const observer = new ResizeObserver((entries) => {
			const entry = entries[entries.length - 1];
			if (entry) {
				setSize({
					width: entry.contentRect.width,
					height: entry.contentRect.height,
				});
			}
		});

		observer.observe(element);
		return () => observer.disconnect();
	}, []);

	const domain = useMemo(() => {
		const finite = series
			.flatMap((s) => s.values)
			.filter(
				(v): v is number => v !== null && v !== undefined && Number.isFinite(v),
			);

		if (finite.length < 2) {
			return null;
		}

		let min = yDomain ? yDomain[0] : Math.min(...finite);
		let max = yDomain ? yDomain[1] : Math.max(...finite);

		if (baseline !== undefined) {
			min = yDomain ? yDomain[0] : Math.min(min, baseline);
			max = yDomain ? yDomain[1] : Math.max(max, baseline);
		}

		if (min === max) {
			min -= 0.5;
			max += 0.5;
		}

		return { min, max };
	}, [series, yDomain, baseline]);

	const ready = domain !== null && size.width > 40 && size.height > 24;

	if (!ready) {
		return (
			<div
				ref={ref}
				className={cn(
					"relative flex min-h-0 min-w-0 items-center justify-center bg-(--bg) font-mono text-[10px] text-(--f4)",
					className,
				)}
			>
				<Scanlines variant="plate" />
				collecting history…
			</div>
		);
	}

	const { min, max } = domain;
	const plotW = Math.max(10, size.width - MARGIN.left - MARGIN.right);
	const plotH = Math.max(10, size.height - MARGIN.top - MARGIN.bottom);
	const span = max - min || 1;
	const spanCount = Math.max(times.length - 1, 1);

	const toPoint = (index: number, value: number): Point => ({
		x: MARGIN.left + (index / spanCount) * plotW,
		y: MARGIN.top + plotH - ((value - min) / span) * plotH,
		value,
	});

	const floorY = MARGIN.top + plotH;
	const baselineY =
		baseline === undefined || baseline < min || baseline > max
			? floorY
			: MARGIN.top + plotH - ((baseline - min) / span) * plotH;

	const yTicks = niceTicks(min, max, 4);
	const xTickCount = Math.min(3, Math.max(times.length, 1));
	const xTicks = Array.from({ length: xTickCount }, (_, i) => {
		const index = Math.round(
			(i / Math.max(xTickCount - 1, 1)) * Math.max(times.length - 1, 0),
		);
		return {
			x: MARGIN.left + (index / spanCount) * plotW,
			label:
				times[index] !== undefined ? formatTime(times[index]) : `#${index + 1}`,
		};
	});

	const labeled = series.filter((s) => s.label !== undefined);
	const chartTitle =
		series
			.map((item) => item.label)
			.filter((label): label is string => Boolean(label))
			.join(" · ") || "training metric";

	return (
		<div ref={ref} className={cn("relative min-h-0 min-w-0", className)}>
			<svg
				width={size.width}
				height={size.height}
				role="img"
				aria-label={chartTitle}
				className="block"
			>
				<title>{chartTitle}</title>
				<defs>
					<pattern
						id={patternId}
						width="20"
						height="20"
						patternUnits="userSpaceOnUse"
					>
						<path
							d="M 20 0 L 0 0 0 20"
							fill="none"
							stroke="var(--line)"
							strokeWidth="0.35"
							opacity="0.4"
						/>
					</pattern>
				</defs>
				<rect
					x={MARGIN.left}
					y={MARGIN.top}
					width={plotW}
					height={plotH}
					fill={`url(#${patternId})`}
				/>
				{yTicks.map((tick) => {
					const y = MARGIN.top + plotH - ((tick - min) / span) * plotH;
					return (
						<g key={`y-${tick}`}>
							<line
								x1={MARGIN.left}
								x2={MARGIN.left + plotW}
								y1={y}
								y2={y}
								stroke="var(--line)"
								strokeWidth="0.5"
								opacity="0.45"
							/>
							<text
								x={MARGIN.left - 4}
								y={y + 3}
								textAnchor="end"
								fill="var(--f4)"
								fontSize="8"
								fontFamily="var(--font-mono, monospace)"
							>
								{formatY(tick)}
							</text>
						</g>
					);
				})}
				<line
					x1={MARGIN.left}
					x2={MARGIN.left}
					y1={MARGIN.top}
					y2={MARGIN.top + plotH}
					stroke="var(--line2)"
				/>
				<line
					x1={MARGIN.left}
					x2={MARGIN.left + plotW}
					y1={MARGIN.top + plotH}
					y2={MARGIN.top + plotH}
					stroke="var(--line2)"
				/>
				{baseline !== undefined && baseline >= min && baseline <= max ? (
					<line
						x1={MARGIN.left}
						x2={MARGIN.left + plotW}
						y1={MARGIN.top + plotH - ((baseline - min) / span) * plotH}
						y2={MARGIN.top + plotH - ((baseline - min) / span) * plotH}
						stroke="var(--f4)"
						strokeDasharray="3 3"
					/>
				) : null}
				{xTicks.map((tick, i) => (
					<g key={`x-${i.toString()}`}>
						<line
							x1={tick.x}
							x2={tick.x}
							y1={MARGIN.top + plotH}
							y2={MARGIN.top + plotH + 4}
							stroke="var(--line2)"
						/>
						<text
							x={tick.x}
							y={size.height - 5}
							textAnchor="middle"
							fill="var(--f4)"
							fontSize="7"
							fontFamily="var(--font-mono, monospace)"
						>
							{tick.label}
						</text>
					</g>
				))}
				{labeled.length > 0 ? (
					<g>
						{labeled.map((s, i) => (
							<g
								key={s.label}
								transform={`translate(${MARGIN.left + 6 + i * 74}, ${MARGIN.top + 8})`}
							>
								<line
									x1={0}
									x2={12}
									y1={0}
									y2={0}
									stroke={s.stroke}
									strokeWidth={2}
								/>
								<text
									x={16}
									y={3}
									fill="var(--f3)"
									fontSize="8"
									fontFamily="var(--font-mono, monospace)"
								>
									{s.label}
								</text>
							</g>
						))}
					</g>
				) : null}
				{series.map((s) => {
					const segments = buildSegments(s.values, toPoint);
					return (
						<g key={s.label ?? s.stroke}>
							{segments
								.filter((seg) => seg.length > 1)
								.map((seg, i) => {
									const line = seg
										.map(
											(p, j) =>
												`${j === 0 ? "M" : "L"}${p.x.toFixed(2)},${p.y.toFixed(2)}`,
										)
										.join(" ");
									const first = seg[0];
									const last = seg[seg.length - 1];
									return (
										<g key={`segment-${i.toString()}`}>
											{s.fill ? (
												<path
													d={`${line} L${last.x.toFixed(2)},${baselineY.toFixed(2)} L${first.x.toFixed(2)},${baselineY.toFixed(2)} Z`}
													fill={s.fill}
													stroke="none"
												/>
											) : null}
											<path
												d={line}
												fill="none"
												stroke={s.stroke}
												strokeWidth={1.5}
											/>
										</g>
									);
								})}
							{segments.length > 0 ? (
								<circle
									cx={segments.at(-1)?.at(-1)?.x}
									cy={segments.at(-1)?.at(-1)?.y}
									r={2.5}
									fill={s.stroke}
								/>
							) : null}
						</g>
					);
				})}
			</svg>
		</div>
	);
};
