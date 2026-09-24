import { curveMonotoneX, line } from "d3-shape";
import { type ComponentProps, useEffect, useRef, useState } from "react";
import { cn } from "#/lib/utils";
import { Flex } from "./flex";

export interface TapePoint {
	x: number;
	y: number;
}
export interface TapeMarker {
	id: string;
	label: string;
	x: number;
	y?: number;
}
export interface TrainingEpisode {
	id: string;
	label?: string;
	points: TapePoint[];
	/** Producer-supplied annotations drawn as hindsight marks. */
	markers?: TapeMarker[];
}
export type EpisodeTapeProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	episode?: TrainingEpisode | null;
	/** The run the tape belongs to, shown as its chip (e.g. the fragment). */
	run?: number;
	/** What the run is doing, e.g. "LEARNING PHASE". */
	phase?: string;
	/** What the coordinate axis counts, e.g. "capture sequence". */
	coordinate?: string;
	/** Hindsight marks by name (A, B, C) at their coordinates. */
	marks?: Record<string, number>;
	/** Coordinates at which the run entered and exited. */
	entered?: number;
	exited?: number;
	/** The confirmed excursion as a signed fraction; its sign names it. */
	outcome?: number;
};

/* Geometry only: the producer owns the observations and annotation coordinates. */
export const projectEpisodeTape = (
	episode: TrainingEpisode,
	width = 600,
	height = 260,
) => {
	const points = episode.points;
	const first = points[0];
	if (!first) return null;
	let minX = first.x;
	let maxX = first.x;
	let minY = first.y;
	let maxY = first.y;
	for (const point of points) {
		minX = Math.min(minX, point.x);
		maxX = Math.max(maxX, point.x);
		minY = Math.min(minY, point.y);
		maxY = Math.max(maxY, point.y);
	}
	const top = 40;
	const bottom = height - 24;
	const x = (value: number) =>
		maxX === minX
			? width / 2
			: 16 + ((value - minX) / (maxX - minX)) * (width - 32);
	const y = (value: number) =>
		maxY === minY
			? (top + bottom) / 2
			: bottom - ((value - minY) / (maxY - minY)) * (bottom - top);
	const path = line<TapePoint>()
		.x((point) => x(point.x))
		.y((point) => y(point.y))
		.curve(curveMonotoneX)(points);
	const within = (value: number) => value >= minX && value <= maxX;
	const at = (value: number) => {
		let nearest = first;
		for (const point of points) {
			if (Math.abs(point.x - value) < Math.abs(nearest.x - value))
				nearest = point;
		}
		return nearest;
	};
	return {
		path: path ?? "",
		x,
		y,
		within,
		at,
		last: points[points.length - 1],
	};
};

/* The size a drawing is given, measured rather than assumed. */
const useMeasured = () => {
	const ref = useRef<HTMLDivElement>(null);
	const [size, setSize] = useState({ width: 600, height: 260 });

	useEffect(() => {
		const target = ref.current;
		if (!target || typeof ResizeObserver === "undefined") return;
		const observer = new ResizeObserver((entries) => {
			const box = entries[0]?.contentRect;
			if (box && box.width > 0 && box.height > 0)
				setSize({ width: box.width, height: box.height });
		});
		observer.observe(target);
		return () => observer.disconnect();
	}, []);

	return { ref, size };
};

const Mark = ({
	x,
	height,
	label,
}: {
	x: number;
	height: number;
	label: string;
}) => (
	<g transform={`translate(${x}, 0)`}>
		<line
			y1={15}
			y2={height}
			stroke="var(--info)"
			strokeDasharray="2 4"
			opacity={0.45}
		/>
		<rect
			x={-7}
			y={8}
			width={14}
			height={14}
			fill="var(--sunken)"
			stroke="var(--info)"
		/>
		<text x={0} y={18} fill="var(--info)" fontSize="9" textAnchor="middle">
			{label}
		</text>
	</g>
);

const Decision = ({
	x,
	y,
	height,
	label,
	tone,
}: {
	x: number;
	y: number;
	height: number;
	label: string;
	tone: string;
}) => (
	<g transform={`translate(${x}, ${y})`}>
		<line y2={height} stroke={tone} opacity={0.3} />
		<circle r={4} fill={tone} />
		<text x={6} y={-6} fill={tone} fontSize="9">
			{label}
		</text>
	</g>
);

export const EpisodeTape = ({
	episode,
	run,
	phase = "LEARNING PHASE",
	coordinate = "capture sequence",
	marks,
	entered,
	exited,
	outcome,
	className,
	...props
}: EpisodeTapeProps) => {
	const { ref, size } = useMeasured();
	const projected =
		episode && size.width > 0
			? projectEpisodeTape(episode, size.width, size.height)
			: null;
	const rising = outcome !== undefined && outcome !== null && outcome > 0;

	return (
		<Flex.Column
			className={cn(
				"min-h-0 min-w-0 overflow-hidden rounded border border-(--line) bg-(--surface) font-mono text-[11px] text-(--f3)",
				className,
			)}
			{...props}
		>
			<Flex.Row className="h-8 shrink-0 items-center justify-between border-(--line) border-b bg-(--sunken) px-4">
				<Flex.Row className="items-center gap-2">
					<span className="font-bold text-(--acc)">RUN</span>
					<span className="rounded border border-(--line) bg-(--raised) px-1.5 py-0.5 text-[10px] text-(--f2)">
						{run === undefined || run === null ? "—" : `EP-${run}`}
					</span>
					<span className="text-(--f4)">›</span>
					<span className="text-(--f1)">{phase}</span>
				</Flex.Row>
				<span className="text-[10px] uppercase tracking-widest text-(--f4)">
					Coordinate
					<span className="ml-1 rounded border border-(--f4) px-1 text-(--f3)">
						{coordinate}
					</span>
				</span>
			</Flex.Row>

			{outcome !== undefined && outcome !== null && (
				<Flex.Row
					className={cn(
						"h-5 shrink-0 items-center border-b px-4 font-bold text-[10px]",
						rising
							? "border-(--up)/20 bg-(--up)/10 text-(--up)"
							: "border-(--down)/20 bg-(--down)/10 text-(--down)",
					)}
				>
					<span>
						{rising ? "UPWARD EXCURSION" : "DOWNWARD EXCURSION"}
						<span className="ml-1 font-normal text-(--f3)">confirmed</span>
					</span>
					<span className="ml-auto">
						{rising ? "+" : ""}
						{(outcome * 100).toFixed(2)}%
					</span>
				</Flex.Row>
			)}

			<div ref={ref} className="relative min-h-0 flex-1 overflow-hidden">
				{!episode?.points.length && (
					<span className="absolute inset-0 flex items-center justify-center text-(--f4) tracking-widest">
						NO EPISODE OBSERVATIONS
					</span>
				)}
				{projected && (
					<svg
						width={size.width}
						height={size.height}
						role="img"
						aria-label={episode?.label ?? "Episode tape"}
						className="absolute inset-0"
					>
						<title>{episode?.label ?? "Episode tape"}</title>
						<line
							x1={0}
							x2={size.width}
							y1={size.height / 2}
							y2={size.height / 2}
							stroke="var(--line)"
							strokeDasharray="2 4"
						/>
						<path
							d={projected.path}
							fill="none"
							stroke="var(--acc)"
							strokeWidth="1.5"
						/>
						<circle
							cx={projected.x(projected.last.x)}
							cy={projected.y(projected.last.y)}
							r="2.5"
							fill="var(--acc)"
						/>
						{Object.entries(marks ?? {})
							.filter(([, at]) => projected.within(at))
							.map(([name, at]) => (
								<Mark
									key={name}
									x={projected.x(at)}
									height={size.height}
									label={name}
								/>
							))}
						{episode?.markers
							?.filter((marker) => projected.within(marker.x))
							.map((marker) => (
								<Mark
									key={marker.id}
									x={projected.x(marker.x)}
									height={size.height}
									label={marker.label}
								/>
							))}
						{entered !== undefined &&
							entered !== null &&
							projected.within(entered) && (
								<Decision
									x={projected.x(entered)}
									y={projected.y(projected.at(entered).y)}
									height={size.height}
									label="ENTER"
									tone="var(--up)"
								/>
							)}
						{exited !== undefined &&
							exited !== null &&
							projected.within(exited) && (
								<Decision
									x={projected.x(exited)}
									y={projected.y(projected.at(exited).y)}
									height={size.height}
									label="EXIT"
									tone="var(--down)"
								/>
							)}
					</svg>
				)}
			</div>
		</Flex.Column>
	);
};
