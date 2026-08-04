import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Toolbar is a horizontal strip of controls and readouts with a rule under it.

It is the sibling of Section.Header, not a duplicate: a Section.Header titles the
region beneath it and puts its meta on the right, while a Toolbar is a row of
peers where nothing is the title. A strip that reads left-to-right as a sentence
of controls belongs here; a pane that needs a name belongs there.

Spacer is a real element rather than `ml-auto` scattered onto whichever child
happened to start the right-hand group, so regrouping the bar does not silently
move the split.
*/

export const toolbarVariants = cva(
	"flex shrink-0 items-center border-(--line) border-b bg-(--surface)",
	{
		variants: {
			size: {
				s: "h-9 gap-2 px-3",
				m: "h-11.5 gap-2 px-3.5",
				lg: "h-13 gap-3.5 px-4",
			},
			/*
				A bar that can outgrow its width scrolls rather than wraps: wrapping
				changes the height of the chrome and shoves the surface below it.
			*/
			overflow: {
				scroll: "overflow-x-auto",
				clip: "overflow-hidden",
			},
		},
		defaultVariants: {
			size: "m",
			overflow: "scroll",
		},
	},
);

export type ToolbarProps = ComponentProps<"header"> &
	VariantProps<typeof toolbarVariants>;

export const Toolbar = ({
	ref,
	size,
	overflow,
	className,
	children,
	...props
}: ToolbarProps) => (
	<header
		ref={ref}
		className={cn(toolbarVariants({ size, overflow }), className)}
		{...props}
	>
		{children}
	</header>
);

Toolbar.Spacer = ({ className, ...props }: ComponentProps<"div">) => (
	<div aria-hidden="true" className={cn("ml-auto", className)} {...props} />
);

/*
Group keeps a cluster of related controls from being split by the bar's own gap
scale — a label and the control it names should sit closer than two unrelated
controls do.
*/
Toolbar.Group = ({
	ref,
	className,
	children,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn("flex shrink-0 items-center gap-2", className)}
		{...props}
	>
		{children}
	</div>
);
