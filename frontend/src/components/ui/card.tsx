import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Card is a region inside a Frame, and CardPanel is its padded interior.

It is deliberately quieter than Panel: a Panel draws its own border because it
sits directly on a surface, while a Card is already inside a plate and only
needs to mark where one region ends and the next begins. Stacked Cards separate
themselves with a rule rather than each drawing a full box.
*/

export const Card = ({ ref, className, ...props }: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"flex min-w-0 flex-col border-(--line) border-b last:border-b-0",
			className,
		)}
		{...props}
	/>
);

export const CardPanel = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => (
	<div ref={ref} className={cn("min-w-0 px-3 py-2.5", className)} {...props} />
);
