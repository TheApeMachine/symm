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
Trail draws a series against a baseline: the part above it is filled with
--up, below with --down, so the shape alone says which side it lives on.
*/
export const Trail = ({ values, baseline = 0, width = 220, height = 48, className }: TrailProps) => {
	const pts = values.filter(Number.isFinite);
	if (pts.length < 2) {
		return (
			<div className={cn("flex items-center justify-center text-[10px] text-(--f4) border border-dashed border-(--line) rounded", className)} style={{ width, height }}>
				collecting history…
			</div>
		);
	}

	const min = Math.min(baseline, ...pts);
	const max = Math.max(baseline, ...pts);
	const span = max - min || 1;
	const y = (v: number) => height - 2 - ((v - min) / span) * (height - 4);
	const x = (i: number) => (i / (pts.length - 1)) * width;
	const line = pts.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
	const area = `${line} L${width},${y(baseline)} L0,${y(baseline)} Z`;
	const id = `trail-${Math.random().toString(36).slice(2)}`;

	return (
		<svg width={width} height={height} className={className} viewBox={`0 0 ${width} ${height}`}>
			<defs>
				<clipPath id={`${id}-a`}><rect x={0} y={0} width={width} height={y(baseline)} /></clipPath>
				<clipPath id={`${id}-b`}><rect x={0} y={y(baseline)} width={width} height={height} /></clipPath>
			</defs>
			<path d={area} fill="color-mix(in srgb, var(--up) 25%, transparent)" clipPath={`url(#${id}-a)`} />
			<path d={area} fill="color-mix(in srgb, var(--down) 25%, transparent)" clipPath={`url(#${id}-b)`} />
			<line x1={0} x2={width} y1={y(baseline)} y2={y(baseline)} stroke="var(--f4)" strokeDasharray="3 3" />
			<path d={line} fill="none" stroke="var(--f1)" strokeWidth={1.4} />
			<circle cx={x(pts.length - 1)} cy={y(pts[pts.length - 1])} r={3} fill={pts[pts.length - 1] >= baseline ? "var(--up)" : "var(--down)"} />
		</svg>
	);
};
