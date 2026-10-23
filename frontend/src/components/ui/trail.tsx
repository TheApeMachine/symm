import { useId } from "react";
import { cn } from "#/lib/utils";

export interface TrailProps {
	values: number[];
	/* Value drawn as the reference line (e.g. 0 for break-even). */
	baseline?: number;
	width?: number;
	height?: number;
	className?: string;
}

/*
Trail draws a series against a baseline. Fill fades from transparent at the
baseline toward soft tone at the edge — same idea as DistanceBar, not a solid
block of red or green.
*/
export const Trail = ({
	values,
	baseline = 0,
	width = 220,
	height = 48,
	className,
}: TrailProps) => {
	const uid = useId().replace(/:/g, "");
	const pts = values.filter(Number.isFinite);
	if (pts.length < 2) {
		return (
			<div
				className={cn(
					"flex items-center justify-center text-[10px] text-(--f4) border border-dashed border-(--line) rounded bg-(--sunken)",
					className,
				)}
				style={{ width, height }}
			>
				collecting history…
			</div>
		);
	}

	const min = Math.min(baseline, ...pts);
	const max = Math.max(baseline, ...pts);
	const span = max - min || 1;
	const y = (v: number) => height - 2 - ((v - min) / span) * (height - 4);
	const x = (i: number) => (i / (pts.length - 1)) * width;
	const line = pts
		.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`)
		.join(" ");
	const area = `${line} L${width},${y(baseline)} L0,${y(baseline)} Z`;
	const yBase = y(baseline);
	const last = pts[pts.length - 1];
	const lastUp = last >= baseline;

	return (
		<svg
			width={width}
			height={height}
			className={cn("rounded border border-(--line) bg-(--sunken)", className)}
			viewBox={`0 0 ${width} ${height}`}
		>
			<title>trail</title>
			<defs>
				<linearGradient
					id={`${uid}-up`}
					x1={0}
					y1={yBase}
					x2={0}
					y2={0}
					gradientUnits="userSpaceOnUse"
				>
					<stop offset="0%" stopColor="var(--up)" stopOpacity={0} />
					<stop offset="100%" stopColor="var(--up)" stopOpacity={0.28} />
				</linearGradient>
				<linearGradient
					id={`${uid}-down`}
					x1={0}
					y1={yBase}
					x2={0}
					y2={height}
					gradientUnits="userSpaceOnUse"
				>
					<stop offset="0%" stopColor="var(--down)" stopOpacity={0} />
					<stop offset="100%" stopColor="var(--down)" stopOpacity={0.22} />
				</linearGradient>
				<clipPath id={`${uid}-a`}>
					<rect x={0} y={0} width={width} height={yBase} />
				</clipPath>
				<clipPath id={`${uid}-b`}>
					<rect x={0} y={yBase} width={width} height={height - yBase} />
				</clipPath>
			</defs>
			<path
				d={area}
				fill={`url(#${uid}-up)`}
				clipPath={`url(#${uid}-a)`}
				opacity={0.85}
			/>
			<path
				d={area}
				fill={`url(#${uid}-down)`}
				clipPath={`url(#${uid}-b)`}
				opacity={0.85}
			/>
			<line
				x1={0}
				x2={width}
				y1={yBase}
				y2={yBase}
				stroke="var(--f4)"
				strokeDasharray="3 3"
				opacity={0.7}
			/>
			<path
				d={line}
				fill="none"
				stroke="var(--f1)"
				strokeWidth={1.4}
				opacity={0.9}
			/>
			<circle
				cx={x(pts.length - 1)}
				cy={y(last)}
				r={3}
				fill={lastUp ? "var(--up)" : "var(--down)"}
				fillOpacity={lastUp ? 0.9 : 0.65}
				stroke="var(--bg)"
				strokeWidth={1.5}
			/>
		</svg>
	);
};
