import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Field is one labelled control's row.

It owns only the spacing between a label and the thing it labels. The control
draws itself; Input in this library deliberately draws no chrome, so a Field
that painted a box would double every edge on the screen.
*/

export const Field = ({ ref, className, ...props }: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn("flex min-w-0 flex-col gap-1", className)}
		{...props}
	/>
);
