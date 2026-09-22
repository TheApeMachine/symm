import { type ComponentProps, useState } from "react";
import { cn } from "@/lib/utils";
import { Flex } from "./flex";
import { Typography } from "./typography";

export interface ImpulsePoint {
	id: number;
	source: string;
	label: string;
	x: number;
	y: number;
	snr: number;
	activation: number;
	energy: number;
	authority: number;
	present: boolean;
}

export interface ImpulseRegion {
	id: number;
	source: string;
	snr: number;
	authority: number;
	members: number;
}

export interface ImpulseContour {
	id: string;
	label: string;
	points: { x: number; y: number }[];
}

export interface ImpulseConnection {
	from: number;
	to: number;
}

export type ImpulseMapProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	points?: ImpulsePoint[];
	regions?: ImpulseRegion[];
	contours?: ImpulseContour[];
	connections?: ImpulseConnection[];
	/** SVG drawing dimensions, in producer coordinate units, centered at the origin. */
	viewport?: { width: number; height: number };
};

/*
Draws supplied positions, contour geometry and relationships without simulating
signals or deciding which evidence is hot. Regions and contours come from the
producer. The view owns hover selection only; empty inputs clear the drawing.
*/
export const ImpulseMap = ({
	points,
	regions,
	contours,
	connections,
	viewport = { width: 640, height: 400 },
	className,
	...props
}: ImpulseMapProps) => {
	const { width, height } = viewport;
	const [selected, setSelected] = useState<number | null>(null);
	const byId = new Map(points?.map((point) => [point.id, point]));
	const inspected = selected === null ? undefined : byId.get(selected);

	return (
		<Flex.Column className={cn("min-h-64 bg-(--sunken)", className)} {...props}>
			<svg
				role="img"
				aria-label="Impulse map"
				className="min-h-0 w-full flex-1"
				viewBox={`${-width / 2} ${-height / 2} ${width} ${height}`}
			>
				{contours?.map((contour) => (
					<polygon
						key={contour.id}
						points={contour.points
							.map((point) => `${point.x},${point.y}`)
							.join(" ")}
						className="fill-(--acc)/10 stroke-(--acc)/40"
						vectorEffect="non-scaling-stroke"
					>
						<title>{contour.label}</title>
					</polygon>
				))}
				{connections?.map((connection) => {
					const from = byId.get(connection.from);
					const to = byId.get(connection.to);

					if (!from || !to)
						throw new Error(
							`ImpulseMap: connection ${connection.from} -> ${connection.to} references a missing point`,
						);

					return (
						<line
							key={`${connection.from}:${connection.to}`}
							x1={from.x}
							y1={from.y}
							x2={to.x}
							y2={to.y}
							className="stroke-(--acc)/50"
							vectorEffect="non-scaling-stroke"
						/>
					);
				})}
				{points?.map((point) => (
					// biome-ignore lint/a11y/useSemanticElements: SVG has no native button; this circle supports click, Enter, Space and focus inspection.
					<circle
						role="button"
						key={point.id}
						cx={point.x}
						cy={point.y}
						r={4}
						className={
							point.present
								? "fill-(--acc) stroke-(--f1)"
								: "fill-transparent stroke-(--f4)"
						}
						vectorEffect="non-scaling-stroke"
						tabIndex={0}
						aria-label={point.label}
						onMouseEnter={() => setSelected(point.id)}
						onMouseLeave={() => setSelected(null)}
						onClick={() => setSelected(point.id)}
						onKeyDown={(event) => {
							if (event.key === "Enter" || event.key === " ") {
								event.preventDefault();
								setSelected(point.id);
							}
						}}
						onFocus={() => setSelected(point.id)}
						onBlur={() => setSelected(null)}
					>
						<title>{`${point.label} · ${point.source} · SNR ${point.snr}`}</title>
					</circle>
				))}
			</svg>
			{!points?.length && (
				<Typography.Mono className="p-3">No impulse data</Typography.Mono>
			)}
			{inspected && (
				<Typography.Mono className="p-3" aria-live="polite">
					{inspected.label} · SNR {inspected.snr} · activation{" "}
					{inspected.activation} · energy {inspected.energy} · authority{" "}
					{inspected.authority}
				</Typography.Mono>
			)}
			{regions?.map((region) => (
				<Flex.Row
					key={region.id}
					className="justify-between border-t border-(--line) p-2"
				>
					<Typography.Label>{region.source}</Typography.Label>
					<Typography.Mono>
						{region.members} members · SNR {region.snr} · authority{" "}
						{region.authority}
					</Typography.Mono>
				</Flex.Row>
			))}
		</Flex.Column>
	);
};
