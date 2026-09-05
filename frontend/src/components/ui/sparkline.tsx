import type { SVGProps } from "react";
import { cn } from "#/lib/utils";

export const computeSparklinePath = (
	points: number[],
	width = 80,
	height = 18,
	padding = 2,
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
		d += (i === 0 ? "M " : " L ") + x.toFixed(1) + "," + y.toFixed(1);
	}

	return d;
};

export type SparklineProps = Omit<SVGProps<SVGSVGElement>, "points"> & {
	points?: number[];
	width?: number;
	height?: number;
	padding?: number;
	strokeColor?: string;
	dataKey?: string;
};

export const Sparkline = ({
	points = [],
	width = 80,
	height = 18,
	padding = 2,
	strokeColor = "var(--acc)",
	dataKey = "sparkline",
	className,
	...props
}: SparklineProps) => {
	const pathD = computeSparklinePath(points, width, height, padding);

	return (
		<svg
			viewBox={`0 0 ${width} ${height}`}
			className={cn("h-4.5 w-full overflow-visible", className)}
			preserveAspectRatio="none"
			{...props}
		>
			<path
				data-k={dataKey}
				d={pathD}
				fill="none"
				stroke={strokeColor}
				strokeWidth="1.5"
				strokeLinecap="round"
				strokeLinejoin="round"
				vectorEffect="non-scaling-stroke"
			/>
		</svg>
	);
};
