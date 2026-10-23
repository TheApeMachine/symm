import { useId, useMemo, useState } from "react";
import { cn } from "#/lib/utils";
import { Scanlines } from "./scanlines";

export type SeriesChartProps = {
	className?: string;
	/** Parallel sample timestamps (ms) for the x-axis */
	times?: number[];
	values: Array<number | null | undefined>;
	formatY?: (value: number) => string;
	/** When set, y-axis uses this domain instead of data min/max */
	yDomain?: [number, number];
	baseline?: number;
	stroke?: string;
	fill?: string;
	height?: number;
	markers?: Array<{ index: number; label: string }>;
	onSelectIndex?: (index: number) => void;
};

const DEFAULT_FORMAT = (value: number) => `${Math.round(value * 100)}%`;

const formatTime = (ms: number) => {
	const d = new Date(ms);
	return d.toLocaleTimeString(undefined, {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
	});
};

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

type Point = { x: number; y: number; index: number; value: number };

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
SeriesChart is one metric on the monitor plate: labeled axes, grid, line, and
a soft fill to the plot floor (or to baseline when given).
*/
export const SeriesChart = ({
	className,
	times,
	values,
	formatY = DEFAULT_FORMAT,
	yDomain,
	baseline,
	stroke = "var(--f1)",
	fill = "color-mix(in srgb, var(--acc) 18%, transparent)",
	height = 100,
	markers = [],
	onSelectIndex,
}: SeriesChartProps) => {
	const patternId = useId();
	const [keyboardIndex, setKeyboardIndex] = useState(0);
	const width = 400;
	const margin = { top: 8, right: 10, bottom: 22, left: 40 };
	const plotW = width - margin.left - margin.right;
	const plotH = height - margin.top - margin.bottom;

	const { min, max, segments, areaPaths, linePaths, yTicks, xTicks, ready } =
		useMemo(() => {
			const finite = values.filter(
				(v): v is number => v !== null && v !== undefined && Number.isFinite(v),
			);

			if (finite.length === 0) {
				return {
					min: 0,
					max: 1,
					segments: [] as Point[][],
					areaPaths: [] as string[],
					linePaths: [] as string[],
					yTicks: [] as number[],
					xTicks: [] as Array<{ x: number; label: string }>,
					ready: false,
				};
			}

			let minV = yDomain ? yDomain[0] : Math.min(...finite);
			let maxV = yDomain ? yDomain[1] : Math.max(...finite);

			if (baseline !== undefined) {
				minV = yDomain ? yDomain[0] : Math.min(minV, baseline);
				maxV = yDomain ? yDomain[1] : Math.max(maxV, baseline);
			}

			if (minV === maxV) {
				minV -= 0.5;
				maxV += 0.5;
			}

			const span = maxV - minV || 1;
			const spanCount = Math.max(values.length - 1, 1);

			const toPoint = (index: number, value: number): Point => ({
				index,
				value,
				x: margin.left + (index / spanCount) * plotW,
				y: margin.top + plotH - ((value - minV) / span) * plotH,
			});

			const segs = buildSegments(values, toPoint);
			const floorY = margin.top + plotH;
			const baselineY =
				baseline === undefined
					? floorY
					: margin.top + plotH - ((baseline - minV) / span) * plotH;

			const areas: string[] = [];
			const lines: string[] = [];

			for (const seg of segs) {
				if (seg.length < 2) {
					continue;
				}

				const line = seg
					.map(
						(p, i) =>
							`${i === 0 ? "M" : "L"}${p.x.toFixed(2)},${p.y.toFixed(2)}`,
					)
					.join(" ");
				lines.push(line);

				const last = seg[seg.length - 1];
				const first = seg[0];
				areas.push(
					`${line} L${last.x.toFixed(2)},${baselineY.toFixed(2)} L${first.x.toFixed(2)},${baselineY.toFixed(2)} Z`,
				);
			}

			const yTickValues = niceTicks(minV, maxV, 4);
			const xTickCount = Math.min(4, Math.max(values.length, 1));
			const xTickIdx =
				values.length <= 1
					? [0]
					: Array.from({ length: xTickCount }, (_, i) =>
							Math.round((i / (xTickCount - 1)) * (values.length - 1)),
						);

			const xT = xTickIdx.map((index) => ({
				x: margin.left + (index / spanCount) * plotW,
				label:
					times && times[index] !== undefined
						? formatTime(times[index])
						: `#${index + 1}`,
			}));

			return {
				min: minV,
				max: maxV,
				segments: segs,
				areaPaths: areas,
				linePaths: lines,
				yTicks: yTickValues,
				xTicks: xT,
				ready: finite.length >= 2,
			};
		}, [
			values,
			times,
			yDomain,
			baseline,
			plotH,
			plotW,
			margin.left,
			margin.top,
		]);

	if (!ready) {
		return (
			<div
				className={cn(
					"relative flex h-full min-h-0 w-full items-center justify-center bg-(--bg) font-mono text-[10px] text-(--f4)",
					className,
				)}
			>
				<Scanlines variant="plate" />
				collecting history…
			</div>
		);
	}

	const span = max - min || 1;
	const yPos = (v: number) => margin.top + plotH - ((v - min) / span) * plotH;

	return (
		<svg
			viewBox={`0 0 ${width} ${height}`}
			preserveAspectRatio="none"
			role={onSelectIndex ? "button" : "img"}
			aria-label={onSelectIndex ? "select a chart sample" : "series chart"}
			tabIndex={onSelectIndex ? 0 : undefined}
			onClick={
				onSelectIndex
					? (event) => {
							const bounds = event.currentTarget.getBoundingClientRect();
							const x =
								((event.clientX - bounds.left) / bounds.width) * width -
								margin.left;
							const index = Math.round(
								(x / plotW) * Math.max(values.length - 1, 1),
							);
							const bounded = Math.min(values.length - 1, Math.max(0, index));
							setKeyboardIndex(bounded);
							onSelectIndex(bounded);
						}
					: undefined
			}
			onKeyDown={
				onSelectIndex
					? (event) => {
							if (event.key === "ArrowLeft") {
								event.preventDefault();
								setKeyboardIndex((index) => Math.max(0, index - 1));
								return;
							}
							if (event.key === "ArrowRight") {
								event.preventDefault();
								setKeyboardIndex((index) =>
									Math.min(values.length - 1, index + 1),
								);
								return;
							}
							if (event.key === "Enter" || event.key === " ") {
								event.preventDefault();
								onSelectIndex(keyboardIndex);
							}
						}
					: undefined
			}
			className={cn(
				"relative z-1 block h-full min-h-0 w-full",
				onSelectIndex && "cursor-crosshair",
				className,
			)}
		>
			<title>series</title>
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
				x={margin.left}
				y={margin.top}
				width={plotW}
				height={plotH}
				fill={`url(#${patternId})`}
			/>
			{/* Y grid + labels */}
			{yTicks.map((tick) => {
				const y = yPos(tick);
				return (
					<g key={`y-${tick}`}>
						<line
							x1={margin.left}
							x2={margin.left + plotW}
							y1={y}
							y2={y}
							stroke="var(--line)"
							strokeWidth="0.5"
							opacity="0.45"
						/>
						<text
							x={margin.left - 4}
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
			{/* Axes */}
			<line
				x1={margin.left}
				x2={margin.left}
				y1={margin.top}
				y2={margin.top + plotH}
				stroke="var(--line2)"
				vectorEffect="non-scaling-stroke"
			/>
			<line
				x1={margin.left}
				x2={margin.left + plotW}
				y1={margin.top + plotH}
				y2={margin.top + plotH}
				stroke="var(--line2)"
				vectorEffect="non-scaling-stroke"
			/>
			{baseline !== undefined && baseline >= min && baseline <= max ? (
				<line
					x1={margin.left}
					x2={margin.left + plotW}
					y1={yPos(baseline)}
					y2={yPos(baseline)}
					stroke="var(--f4)"
					strokeDasharray="3 3"
					vectorEffect="non-scaling-stroke"
				/>
			) : null}
			{/* X ticks */}
			{xTicks.map((tick) => (
				<g key={`x-${tick.label}`}>
					<line
						x1={tick.x}
						x2={tick.x}
						y1={margin.top + plotH}
						y2={margin.top + plotH + 4}
						stroke="var(--line2)"
					/>
					<text
						x={tick.x}
						y={height - 4}
						textAnchor="middle"
						fill="var(--f4)"
						fontSize="7"
						fontFamily="var(--font-mono, monospace)"
					>
						{tick.label}
					</text>
				</g>
			))}
			{areaPaths.map((d) => (
				<path key={d} d={d} fill={fill} stroke="none" />
			))}
			{linePaths.map((d) => (
				<path
					key={d}
					d={d}
					fill="none"
					stroke={stroke}
					strokeWidth={1.5}
					vectorEffect="non-scaling-stroke"
				/>
			))}
			{markers.map((marker) => {
				const x =
					margin.left + (marker.index / Math.max(values.length - 1, 1)) * plotW;
				return (
					<g key={`${marker.label}-${marker.index}`}>
						<line
							x1={x}
							x2={x}
							y1={margin.top}
							y2={margin.top + plotH}
							stroke="var(--acc)"
							strokeDasharray="3 2"
							vectorEffect="non-scaling-stroke"
						/>
						<text
							x={x + 2}
							y={margin.top + 8}
							fill="var(--acc)"
							fontSize="8"
							fontFamily="var(--font-mono, monospace)"
						>
							{marker.label}
						</text>
					</g>
				);
			})}
			{segments.flat().length > 0 ? (
				<circle
					cx={segments.at(-1)?.at(-1)?.x}
					cy={segments.at(-1)?.at(-1)?.y}
					r={2.5}
					fill={stroke}
				/>
			) : null}
		</svg>
	);
};
