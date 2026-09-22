import { curveMonotoneX, line } from "d3-shape";
import type { ComponentProps } from "react";
import { Flex } from "./flex";
import { Typography } from "./typography";

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
	/** Producer-supplied annotations, including A/B/C and policy actions. */
	markers?: TapeMarker[];
}
export type EpisodeTapeProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	episode?: TrainingEpisode | null;
};

/* Geometry only: the producer owns the observations and annotation coordinates. */
export const projectEpisodeTape = (episode: TrainingEpisode) => {
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
	// Drawing coordinates: 600 by 260, with space for labels at the borders.
	const x = (value: number) =>
		maxX === minX ? 300 : 20 + ((value - minX) / (maxX - minX)) * 560;
	const y = (value: number) =>
		maxY === minY ? 130 : 236 - ((value - minY) / (maxY - minY)) * 212;
	const path = line<TapePoint>()
		.x((point) => x(point.x))
		.y((point) => y(point.y))
		.curve(curveMonotoneX)(points);
	return { path: path ?? "", x, y, last: points[points.length - 1] };
};

export const EpisodeTape = ({ episode, ...props }: EpisodeTapeProps) => {
	const projected = episode ? projectEpisodeTape(episode) : null;
	return (
		<Flex.Column {...props}>
			<Typography.Label>{episode?.label ?? "Episode tape"}</Typography.Label>
			{!projected && <Typography.Mono>No episode observations</Typography.Mono>}
			{projected && (
				<svg
					viewBox="0 0 600 260"
					role="img"
					aria-label={episode?.label ?? "Episode tape"}
					className="min-h-48 w-full flex-1"
				>
					<title>{episode?.label ?? "Episode tape"}</title>
					<path
						d={projected.path}
						fill="none"
						stroke="var(--acc)"
						strokeWidth="1.6"
					/>
					<circle
						cx={projected.x(projected.last.x)}
						cy={projected.y(projected.last.y)}
						r="3"
						fill="var(--acc)"
					/>
					{episode?.markers?.map((marker) => (
						<g key={marker.id}>
							<line
								x1={projected.x(marker.x)}
								x2={projected.x(marker.x)}
								y1="20"
								y2="240"
								stroke="var(--info)"
								strokeDasharray="2 3"
							/>
							{marker.y !== undefined && (
								<circle
									cx={projected.x(marker.x)}
									cy={projected.y(marker.y)}
									r="4"
									fill="var(--info)"
								/>
							)}
							<text
								x={projected.x(marker.x)}
								y="14"
								textAnchor="middle"
								fontSize="10"
								fill="var(--info)"
							>
								{marker.label}
							</text>
						</g>
					))}
				</svg>
			)}
		</Flex.Column>
	);
};
