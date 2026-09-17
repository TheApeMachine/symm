import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
DistributionCurve renders a normal probability density curve (Gaussian fit)
from measured mean and standard deviation.

It displays a base axis, breakeven reference line, mean indicator, and
an area fill shaded using the semantic tone variable.
*/

export type DistributionCurveProps = Omit<ComponentProps<"svg">, "children"> & {
	mean: number;
	sd: number;
	min?: number;
	max?: number;
	breakeven?: number;
	width?: number;
	height?: number;
	tone?: "acc" | "info" | "brand" | "up" | "down";
	unit?: string;
};

export const computeDistributionPath = (
	mean: number,
	sd: number,
	min: number,
	max: number,
	width: number,
	height: number,
	padX = 20,
	padTop = 20,
	padBottom = 25,
): { linePath: string; areaPath: string; meanX: number; zeroX: number } => {
	const safeSd = Math.max(0.1, sd);
	const usableWidth = Math.max(1, width - padX * 2);
	const usableHeight = Math.max(1, height - padTop - padBottom);
	const baseLineY = height - padBottom;

	const steps = 60;
	const stepVal = (max - min) / steps;
	const points: { x: number; y: number }[] = [];

	let maxY = 0;
	for (let i = 0; i <= steps; i++) {
		const val = min + i * stepVal;
		const pdf =
			(1 / (safeSd * Math.sqrt(2 * Math.PI))) *
			Math.exp(-0.5 * Math.pow((val - mean) / safeSd, 2));
		if (pdf > maxY) maxY = pdf;
		points.push({ x: val, y: pdf });
	}

	const safeMaxY = maxY > 0 ? maxY * 1.15 : 1;

	const coords = points.map((pt) => ({
		cx: padX + ((pt.x - min) / (max - min)) * usableWidth,
		cy: baseLineY - (pt.y / safeMaxY) * usableHeight,
	}));

	let linePath = "";
	for (let i = 0; i < coords.length; i++) {
		const prefix = i === 0 ? "M " : " L ";
		linePath += `${prefix}${coords[i].cx.toFixed(1)},${coords[i].cy.toFixed(1)}`;
	}

	const firstX = coords[0]?.cx ?? padX;
	const lastX = coords[coords.length - 1]?.cx ?? width - padX;
	const areaPath = `${linePath} L ${lastX.toFixed(1)},${baseLineY} L ${firstX.toFixed(1)},${baseLineY} Z`;

	const meanClamped = Math.max(min, Math.min(max, mean));
	const meanX = padX + ((meanClamped - min) / (max - min)) * usableWidth;
	const zeroClamped = Math.max(min, Math.min(max, 0));
	const zeroX = padX + ((zeroClamped - min) / (max - min)) * usableWidth;

	return { linePath, areaPath, meanX, zeroX };
};

export const DistributionCurve = ({
	mean,
	sd,
	min = -15,
	max = 15,
	breakeven = 0,
	width = 320,
	height = 140,
	tone = "info",
	unit = "bp",
	className,
	...props
}: DistributionCurveProps) => {
	const { linePath, areaPath, meanX, zeroX } = computeDistributionPath(
		mean,
		sd,
		min,
		max,
		width,
		height,
	);

	const strokeColor = `var(--${tone})`;
	const fillId = `dist-gradient-${tone}`;

	return (
		<svg
			viewBox={`0 0 ${width} ${height}`}
			className={cn("w-full h-full font-mono text-[9px]", className)}
			{...props}
		>
			<defs>
				<linearGradient id={fillId} x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" stopColor={strokeColor} stopOpacity="0.25" />
					<stop offset="100%" stopColor={strokeColor} stopOpacity="0.0" />
				</linearGradient>
			</defs>

			{/* X Base Axis */}
			<line
				x1={20}
				y1={height - 25}
				x2={width - 20}
				y2={height - 25}
				stroke="var(--line)"
				strokeWidth={1}
			/>

			{/* Breakeven Marker (0.0) */}
			{breakeven >= min && breakeven <= max && (
				<>
					<line
						x1={zeroX}
						y1={20}
						x2={zeroX}
						y2={height - 25}
						stroke="var(--line2)"
						strokeWidth={1}
						strokeDasharray="2 2"
					/>
					<text
						x={zeroX}
						y={14}
						fill="var(--f4)"
						fontSize="9px"
						textAnchor="middle"
					>
						{breakeven.toFixed(1)} {unit}
					</text>
				</>
			)}

			{/* Mean Marker Line */}
			<line
				x1={meanX}
				y1={20}
				x2={meanX}
				y2={height - 25}
				stroke={mean >= 0 ? "var(--up)" : "var(--down)"}
				strokeWidth={1.2}
			/>
			<text
				x={meanX}
				y={14}
				fill={mean >= 0 ? "var(--up)" : "var(--down)"}
				fontSize="9px"
				fontWeight="bold"
				textAnchor="middle"
			>
				μ {mean > 0 ? `+${mean.toFixed(1)}` : mean.toFixed(1)}
			</text>

			{/* Density Area & Line */}
			<path d={areaPath} fill={`url(#${fillId})`} />
			<path
				d={linePath}
				fill="none"
				stroke={strokeColor}
				strokeWidth={1.5}
				strokeLinecap="round"
			/>

			{/* Axis Bounds Labels */}
			<text x={20} y={height - 10} fill="var(--f4)" fontSize="9px">
				{min} {unit}
			</text>
			<text
				x={width - 20}
				y={height - 10}
				fill="var(--f4)"
				fontSize="9px"
				textAnchor="end"
			>
				+{max} {unit}
			</text>
		</svg>
	);
};
