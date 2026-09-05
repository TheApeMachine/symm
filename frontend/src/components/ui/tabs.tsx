import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

/*
Tabs is the segmented control: a recessed track holding one raised plate.

The depth is the whole idea, and it is why this is not a row of buttons. The
track sits on --sunken behind a hairline, one step *below* the surface it is
drawn on; the selected tab sits on --raised, one step above it. So the control
reads as a physical switch with exactly one key pressed, and the selected state
survives without relying on colour alone — which a row of solid-vs-quiet buttons
does not, and which is why those read as decoration rather than as a position.

Reach for Tabs when the options are mutually exclusive views of one subject and
exactly one is always active. A row of Buttons is for actions, and a Badge is
for what a thing *is* — using either to show which view you are on is how a
selected state stops looking selected.
*/

export const tabsVariants = cva(
	"inline-flex items-center rounded-[3px] border border-(--line) bg-(--sunken) p-0.5",
	{
		variants: {
			size: {
				xs: "gap-px text-[9px]",
				s: "gap-px text-[10px]",
				m: "gap-0.5 text-[11px]",
				lg: "gap-0.5 text-xs",
			},
			fullWidth: {
				true: "flex w-full",
			},
			orientation: {
				horizontal: "",
				/*
					A vertical rail is the same control turned: the modal that this
					pattern came from ran its views down the left edge.
				*/
				vertical: "flex-col items-stretch",
			},
		},
		defaultVariants: {
			size: "s",
			orientation: "horizontal",
		},
	},
);

/*
Tone picks what the pressed plate is tinted with. Accent is the default because
a view switcher is chrome, not status: the semantic tones are reserved for
controls where the colour is telling you something about the data.
*/
export const tabVariants = cva(
	[
		"cursor-pointer select-none whitespace-nowrap rounded-[2px] font-mono uppercase",
		"transition-colors duration-150",
		"[--tab-tone:var(--acc)]",
		"focus-visible:outline focus-visible:outline-1 focus-visible:outline-(--tab-tone) focus-visible:outline-offset-1",
	],
	{
		variants: {
			size: {
				xs: "px-1.5 py-0.5 tracking-[0.08em]",
				s: "px-2.5 py-1 tracking-[0.08em]",
				m: "px-3 py-1 tracking-[0.07em]",
				lg: "px-3.5 py-1.5 tracking-[0.06em]",
			},
			tone: {
				accent: "[--tab-tone:var(--acc)]",
				info: "[--tab-tone:var(--info)]",
				success: "[--tab-tone:var(--success)]",
				warning: "[--tab-tone:var(--warning)]",
				error: "[--tab-tone:var(--error)]",
			},
			active: {
				true: "bg-(--raised) font-semibold text-(--tab-tone) shadow-xs",
				false: "text-(--f4) hover:text-(--f2)",
			},
			grow: {
				true: "flex-1 text-center",
			},
		},
		defaultVariants: {
			size: "s",
			tone: "accent",
			active: false,
		},
	},
);

export type TabsProps = Omit<ComponentProps<"div">, "onSelect"> &
	VariantProps<typeof tabsVariants>;

export const Tabs = ({
	ref,
	size,
	orientation,
	fullWidth,
	className,
	children,
	...props
}: TabsProps) => (
	<div
		ref={ref}
		role="tablist"
		aria-orientation={orientation ?? "horizontal"}
		className={cn(tabsVariants({ size, orientation, fullWidth }), className)}
		{...props}
	>
		{children}
	</div>
);

export type TabProps = ComponentProps<"button"> &
	Omit<VariantProps<typeof tabVariants>, "active"> & {
		active?: boolean;
		/*
			A label may be a painted node rather than a literal, so a live value can
			sit in a tab without the control re-rendering.
		*/
		children?: ReactNode;
	};

/*
Tab is a real button with role="tab", so the control is reachable and its state
is announced. `aria-selected` is what carries the selection to assistive tech —
the raised plate carries it to everyone else.
*/
Tabs.Tab = ({
	ref,
	size,
	tone,
	active = false,
	grow,
	className,
	children,
	...props
}: TabProps) => (
	<button
		ref={ref}
		type="button"
		role="tab"
		aria-selected={active}
		className={cn(tabVariants({ size, tone, active, grow }), className)}
		{...props}
	>
		{children}
	</button>
);
