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
		d += `${(i === 0 ? "M " : " L ") + x.toFixed(1)},${y.toFixed(1)}`;
	}

	return d;
};

export type SparklinePaths = {
	spark: string;
	area: string;
	line: string;
	fill: string;
	active: boolean;
};

export const computeSparklinePaths = (
	values: number[],
	active = true,
): SparklinePaths => {
	let minimum = values[0];
	let maximum = values[0];

	for (const value of values) {
		minimum = Math.min(minimum, value);
		maximum = Math.max(maximum, value);
	}
	const range = maximum - minimum;
	// These dimensions match the SVG viewBox, with y coordinates from 3 to 29.
	const points = values.map((value, index) => {
		const x = values.length === 1 ? 75 : (index / (values.length - 1)) * 150;
		const y = range === 0 ? 16 : 29 - ((value - minimum) / range) * 26;
		return `${x.toFixed(1)},${y.toFixed(1)}`;
	});
	const spark = points.join(" ");
	const area = points.length > 0 ? `${spark} 150,30 0,30` : "";

	return {
		spark,
		area,
		line: active ? "var(--acc)" : "var(--info)",
		fill: active
			? "color-mix(in srgb, var(--acc) 16%, transparent)"
			: "color-mix(in srgb, var(--info) 12%, transparent)",
		active,
	};
};

/*
setSparkline updates sparkline polyline elements directly without triggering React re-renders.
Accepts either the svg root or the spark and area elements directly.
*/
export const setSparkline = (
	sparkOrSvg: SVGElement | HTMLElement | null | undefined,
	areaOrValues: SVGElement | HTMLElement | number[] | null | undefined,
	valuesOrActive?: number[] | boolean,
	active = true,
) => {
	let sparkEl: SVGPolylineElement | null = null;
	let areaEl: SVGPolylineElement | null = null;
	let values: number[] = [];
	let isActive = active;

	if (Array.isArray(areaOrValues)) {
		const root = sparkOrSvg as HTMLElement | SVGElement | null | undefined;
		if (!root) return;
		sparkEl = (
			root.matches?.('[data-k="spark"]')
				? root
				: root.querySelector('[data-k="spark"]')
		) as SVGPolylineElement | null;
		areaEl = (
			root.matches?.('[data-k="area"]')
				? root
				: root.querySelector('[data-k="area"]')
		) as SVGPolylineElement | null;
		values = areaOrValues;
		isActive = typeof valuesOrActive === "boolean" ? valuesOrActive : true;
	} else {
		sparkEl = sparkOrSvg as SVGPolylineElement | null;
		areaEl = areaOrValues as SVGPolylineElement | null;
		values = Array.isArray(valuesOrActive) ? valuesOrActive : [];
	}

	const paths = computeSparklinePaths(values, isActive);

	if (sparkEl) {
		sparkEl.setAttribute("points", paths.spark);
		sparkEl.setAttribute("stroke", paths.line);
	}

	if (areaEl) {
		areaEl.setAttribute("points", paths.area);
		areaEl.setAttribute("fill", paths.fill);
	}
};

export type SparklineProps = Omit<SVGProps<SVGSVGElement>, "points"> & {
	points?: number[] | null;
	title?: string;
	active?: boolean;
};

export const Sparkline = ({
	points = [],
	title = "Sparkline",
	active = false,
	className,
	...props
}: SparklineProps) => {
	const paths = computeSparklinePaths(points ?? [], active);

	return (
		<svg
			viewBox="0 0 150 30"
			preserveAspectRatio="none"
			className={cn("mt-1 block min-h-2.5 w-full flex-1", className)}
			aria-label={title}
			{...props}
		>
			<title>{title}</title>
			<polyline
				data-k="area"
				points={paths.area}
				fill={paths.fill}
				stroke="none"
			/>
			<polyline
				data-k="spark"
				points={paths.spark}
				fill="none"
				stroke={paths.line}
				strokeWidth="1.4"
				vectorEffect="non-scaling-stroke"
			/>
		</svg>
	);
};
