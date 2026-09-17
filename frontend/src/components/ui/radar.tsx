import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

export const radarVariants = cva("block w-full", {
	variants: {
		variant: {
			warning: "[--radar-arm:var(--acc)]",
			brand: "[--radar-arm:var(--brand)]",
			info: "[--radar-arm:var(--info)]",
			success: "[--radar-arm:var(--success)]",
			error: "[--radar-arm:var(--error)]",
		},
		size: {
			s: "max-w-[180px]",
			m: "max-w-[220px]",
			lg: "max-w-[280px]",
			full: "w-full",
		},
	},
	defaultVariants: {
		variant: "warning",
		size: "full",
	},
});

export type RadarAxis = {
	label: string;
	x: number;
	y: number;
	initialValue?: number;
};

export type RadarProps = Omit<ComponentPropsWithoutRef<"svg">, "children"> &
	VariantProps<typeof radarVariants> & {
		axes: RadarAxis[];
		title?: string;
		radius?: number;
		labelRadius?: number;
		center?: { x: number; y: number };
		levels?: Array<{ radiusRatio: number; stroke: string }>;
		sweep?: boolean;
	};

const DEFAULT_LEVELS = [
	{ radiusRatio: 1.0, stroke: "var(--line2)" },
	{ radiusRatio: 0.67, stroke: "var(--line)" },
	{ radiusRatio: 0.33, stroke: "var(--line)" },
];

/**
 * Radar renders a high-precision polygon radar chart with concentric guide rings,
 * radial spokes, dynamic value arms scaled via CSS `--axis` variables (or direct DOM),
 * and axis labels.
 */
export const Radar = ({
	axes,
	title = "Regime radar",
	radius = 84,
	labelRadius = 98,
	center = { x: 110, y: 105 },
	levels = DEFAULT_LEVELS,
	sweep = false,
	variant,
	size,
	className,
	...props
}: RadarProps) => {
	const computePolygonPoints = (scaleRatio: number) => {
		return axes
			.map((axis) => {
				const x = (center.x + axis.x * radius * scaleRatio).toFixed(1);
				const y = (center.y + axis.y * radius * scaleRatio).toFixed(1);
				return `${x},${y}`;
			})
			.join(" ");
	};

	return (
		<svg
			viewBox="0 0 220 210"
			className={cn(radarVariants({ variant, size }), className)}
			aria-label={title}
			{...props}
		>
			<title>{title}</title>

			{/* Background grid concentric rings */}
			{levels.map((level, index) => (
				<polygon
					key={`level:${index}`}
					points={computePolygonPoints(level.radiusRatio)}
					fill="none"
					stroke={level.stroke}
					strokeWidth="1"
				/>
			))}

			{/* Radial spokes */}
			{axes.map((axis) => (
				<line
					key={`spoke:${axis.label}`}
					x1={center.x}
					y1={center.y}
					x2={center.x + axis.x * radius}
					y2={center.y + axis.y * radius}
					stroke="var(--line)"
					strokeWidth="1"
				/>
			))}

			{/* Subtle scanning sweep animation if enabled */}
			{sweep && (
				<circle
					cx={center.x}
					cy={center.y}
					r={radius}
					fill="none"
					stroke="var(--radar-arm)"
					strokeWidth="0.75"
					strokeDasharray="4 6"
					className="animate-[spin_8s_linear_infinite] opacity-25"
					style={{ transformOrigin: `${center.x}px ${center.y}px` }}
				/>
			)}

			{/* Dynamic arms (scaled directly via CSS --axis or DOM mutations) */}
			{axes.map((axis) => {
				const initialAxis = axis.initialValue !== undefined ? axis.initialValue : 0;
				return (
					<g
						key={`arm:${axis.label}`}
						data-axis={axis.label}
						style={{
							transform: `scale(clamp(0, var(--axis, ${initialAxis}), 1))`,
							transformOrigin: `${center.x}px ${center.y}px`,
							transition: "transform 0.15s cubic-bezier(0.16, 1, 0.3, 1)",
						}}
					>
						<line
							x1={center.x}
							y1={center.y}
							x2={center.x + axis.x * radius}
							y2={center.y + axis.y * radius}
							stroke="var(--radar-arm)"
							strokeWidth="1.6"
						/>
						<circle
							cx={center.x + axis.x * radius}
							cy={center.y + axis.y * radius}
							r="2.6"
							fill="var(--radar-arm)"
						/>
					</g>
				);
			})}

			{/* Axis labels */}
			{axes.map((axis) => (
				<text
					key={`label:${axis.label}`}
					x={center.x + axis.x * labelRadius}
					y={center.y + axis.y * labelRadius + 3}
					textAnchor="middle"
					fontSize="9"
					className="font-mono fill-(--f3) select-none"
				>
					{axis.label}
				</text>
			))}
		</svg>
	);
};
