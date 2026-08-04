import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Scanlines } from "./scanlines";
import { Typography } from "./typography";

/*
Canvas is the frame around a plot: a dark plate with the drawing surface filling
it edge to edge, and chrome floating over the corners rather than boxed above it.

Every piece of that chrome is pointer-events-none. The plot underneath is often
interactive — dragged, hovered, picked — and a caption that quietly ate a corner
of those events would be near impossible to trace back to the caption.

The plate is --sunken rather than a literal, so a re-themed host gets a plot
background that still matches its own darkest surface.
*/

export type CanvasProps = Omit<ComponentProps<"div">, "title"> & {
	title: ReactNode;
	/* The line under the title: what is being plotted, and how. */
	meta?: ReactNode;
	/* Corner slots. Legend is unwrapped so it can place itself. */
	topRight?: ReactNode;
	legend?: ReactNode;
	footer?: ReactNode;
	scanlines?: boolean;
	children: ReactNode;
};

export const Canvas = ({
	ref,
	title,
	meta,
	topRight,
	legend,
	footer,
	scanlines = true,
	className,
	children,
	...props
}: CanvasProps) => (
	<div
		ref={ref}
		className={cn("relative min-h-0 overflow-hidden bg-(--sunken)", className)}
		{...props}
	>
		<div className="absolute inset-0">{children}</div>

		{scanlines ? <Scanlines variant="plate" /> : null}

		<div className="pointer-events-none absolute top-2.75 left-3">
			<Typography.Label size="m" tone="f2">
				{title}
			</Typography.Label>
			{meta === undefined ? null : (
				<Typography.Mono size="xs" tone="f4" className="mt-0.5 block">
					{meta}
				</Typography.Mono>
			)}
		</div>

		{topRight === undefined ? null : (
			<div className="pointer-events-none absolute top-2.75 right-3 text-right font-mono text-[9.5px] text-(--f3) leading-[1.6]">
				{topRight}
			</div>
		)}

		{legend}

		{footer === undefined ? null : (
			<div className="pointer-events-none absolute right-3 bottom-2 font-mono text-[9.5px] text-(--f3)">
				{footer}
			</div>
		)}
	</div>
);
