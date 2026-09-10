import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Fieldset groups the fields that belong to one thing, under a Legend naming it.

Where Field is a single control's row, a Fieldset is the set — a node's inputs,
a node's outputs. The legend is the group's name rather than a heading, so it
takes the overline treatment the rest of this library gives to rail titles.
*/

export const Fieldset = ({
	ref,
	className,
	...props
}: ComponentProps<"fieldset">) => (
	<fieldset
		ref={ref}
		className={cn("flex min-w-0 flex-col gap-1.5 border-0 p-0", className)}
		{...props}
	/>
);

Fieldset.Legend = ({ ref, className, ...props }: ComponentProps<"legend">) => (
	<legend
		ref={ref}
		className={cn(
			"px-0 text-[9px] text-(--f4) uppercase tracking-[0.14em]",
			className,
		)}
		{...props}
	/>
);
