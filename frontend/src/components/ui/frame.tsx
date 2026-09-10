import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Frame is a plate that stands on its own on a surface it does not fill.

Panel is the bordered box inside a laid-out pane; a Frame is a thing placed on
a canvas — a node on a graph, a card floating over a stage — so it carries the
shadow and the slightly heavier edge that says it is above the plane rather
than cut into it. Its parts are ordered: Header, then whatever body the caller
composes, then Footer, separated by rules rather than by gaps so the plate
reads as one object.
*/

export const frameVariants = cva(
	[
		"flex min-w-0 flex-col overflow-hidden rounded-[4px]",
		// Stated, not inherited: a Frame may be portaled clear of the page.
		"border border-(--line2) bg-(--surface) text-(--f2)",
		"shadow-[0_8px_24px_-12px_rgb(0_0_0/0.75)]",
	],
	{
		variants: {
			tone: {
				default: "",
				accent: "border-(--acc)",
				error: "border-(--error)",
			},
		},
		defaultVariants: {
			tone: "default",
		},
	},
);

export type FrameProps = ComponentProps<"div"> &
	VariantProps<typeof frameVariants>;

export const Frame = ({ ref, tone, className, ...props }: FrameProps) => (
	<div
		ref={ref}
		className={cn(frameVariants({ tone }), className)}
		{...props}
	/>
);

export const FrameHeader = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"flex shrink-0 flex-col gap-1 border-(--line) border-b px-3 py-2",
			className,
		)}
		{...props}
	/>
);

/*
Title and Description are spans rather than headings: a Frame is placed, not
sectioned, and a canvas full of h3s is a document outline nobody asked for.
*/
export const FrameTitle = ({
	ref,
	className,
	...props
}: ComponentProps<"span">) => (
	<span
		ref={ref}
		className={cn(
			"truncate font-medium text-[12px] text-(--f1) leading-tight",
			className,
		)}
		{...props}
	/>
);

export const FrameDescription = ({
	ref,
	className,
	...props
}: ComponentProps<"span">) => (
	<span
		ref={ref}
		className={cn("truncate text-[10.5px] text-(--f3) leading-snug", className)}
		{...props}
	/>
);

export const FrameFooter = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"flex shrink-0 items-center gap-2 border-(--line) border-t px-3 py-2",
			className,
		)}
		{...props}
	/>
);
