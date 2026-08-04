import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Scanlines is the CRT raster this UI is drawn on.

It was written twice with different periods and different blend modes — once
over the whole viewport, once inside a plotting surface — so the two never quite
matched. They are one component with two variants now.

`screen` lays a 4px raster over everything and takes its strength from --scan,
so a host app can dim or kill it with a single custom property. `plate` is the
tighter 3px raster that belongs inside a chart, multiplied so it darkens the
plot without washing out the line colours over it.

Always inert: pointer-events-none and aria-hidden, never in the tab order.
*/

const RASTER: Record<"screen" | "plate", string> = {
	screen:
		"repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(0,0,0,0.18) 2px, rgba(0,0,0,0.18) 4px)",
	plate:
		"repeating-linear-gradient(0deg, rgba(0,0,0,0.18) 0px, rgba(0,0,0,0.18) 1px, transparent 1px, transparent 3px)",
};

export const scanlinesVariants = cva("pointer-events-none", {
	variants: {
		variant: {
			screen: "fixed inset-0 opacity-(--scan)",
			plate: "absolute inset-0 opacity-50 mix-blend-multiply",
		},
	},
	defaultVariants: {
		variant: "plate",
	},
});

export type ScanlinesProps = Omit<ComponentProps<"div">, "children"> &
	VariantProps<typeof scanlinesVariants>;

export const Scanlines = ({
	ref,
	variant,
	className,
	style,
	...props
}: ScanlinesProps) => (
	<div
		ref={ref}
		aria-hidden="true"
		className={cn(scanlinesVariants({ variant }), className)}
		style={{ backgroundImage: RASTER[variant ?? "plate"], ...style }}
		{...props}
	/>
);
