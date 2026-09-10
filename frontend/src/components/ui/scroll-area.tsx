import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
ScrollArea is a region that scrolls without drawing a scrollbar over the
content.

It stays native rather than emulating scrolling in JavaScript: a custom
scrollbar loses momentum, keyboard paging, and the browser's own overscroll
behaviour, and every list in this app is inside something that already
scrolls. Only the bar's appearance is taken over, so a thin rule sits against
the terminal's surfaces instead of a system-blue slab.
*/

export type ScrollAreaProps = ComponentProps<"div"> & {
	orientation?: "vertical" | "horizontal" | "both";
};

export const ScrollArea = ({
	ref,
	orientation = "vertical",
	className,
	...props
}: ScrollAreaProps) => (
	<div
		ref={ref}
		className={cn(
			"min-h-0 min-w-0",
			orientation === "vertical" && "overflow-y-auto overflow-x-hidden",
			orientation === "horizontal" && "overflow-x-auto overflow-y-hidden",
			orientation === "both" && "overflow-auto",
			"[scrollbar-color:var(--line2)_transparent] [scrollbar-width:thin]",
			"[&::-webkit-scrollbar]:size-1.5",
			"[&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-(--line2)",
			"[&::-webkit-scrollbar-track]:bg-transparent",
			className,
		)}
		{...props}
	/>
);
