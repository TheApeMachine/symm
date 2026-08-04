import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Sparkline is a paint target, not a chart.

It renders no data. It renders the two shapes — a filled area and a stroked
line — that Component's data-append writer pushes points into as frames arrive,
and it publishes the geometry that writer needs to lay those points out. React
never sees a value; the websocket writes the `points` attribute directly and the
tree never re-renders. That is the whole reason this is worth having as a
component: the contract between the markup and the writer is four attributes
that must agree, and agreeing by hand at every call site is how a sparkline ends
up plotting into a box it was never sized for.

	<Sparkline bind="metrics.conditional_intensity.raw" />

`bind` is the path inside the painted frame. `width` and `height` are the user
units the writer plots into and the viewBox reads back — they are not display
size, which comes from className, because preserveAspectRatio="none" lets the
same 150x30 series stretch to whatever box it is given.

Values are expected in 0..1; the writer clamps to that range.
*/

export type SparklineProps = Omit<
	ComponentProps<"svg">,
	"children" | "width" | "height"
> & {
	/* Path inside the painted frame, written to data-append. */
	bind: string;
	/* Plot-space geometry the append writer shares with the viewBox. */
	width?: number;
	height?: number;
	/* How many samples are retained before the oldest is dropped. */
	limit?: number;
	/* Draw the filled area beneath the line. */
	area?: boolean;
	stroke?: string;
	fill?: string;
	strokeWidth?: number;
	title?: string;
};

export const Sparkline = ({
	ref,
	bind,
	width = 150,
	height = 30,
	limit = 40,
	area = true,
	stroke = "var(--acc)",
	fill = "color-mix(in srgb, var(--acc) 16%, transparent)",
	strokeWidth = 1.4,
	title = "Trace",
	className,
	...props
}: SparklineProps) => {
	const geometry = {
		"data-append": bind,
		"data-target": "points",
		"data-append-limit": String(limit),
		"data-append-width": String(width),
		"data-append-height": String(height),
	};

	return (
		<svg
			ref={ref}
			viewBox={`0 0 ${width} ${height}`}
			preserveAspectRatio="none"
			className={cn("block w-full", className)}
			aria-hidden="true"
			{...props}
		>
			<title>{title}</title>
			{area ? <polygon {...geometry} fill={fill} stroke="none" /> : null}
			<polyline
				{...geometry}
				fill="none"
				stroke={stroke}
				strokeWidth={strokeWidth}
				vectorEffect="non-scaling-stroke"
			/>
		</svg>
	);
};
