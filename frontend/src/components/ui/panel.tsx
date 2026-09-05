import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Flex } from "./flex";

export const panelVariants = cva("border border-(--line)", {
	variants: {
		variant: {
			sunken: "bg-(--sunken)",
			surface: "bg-(--surface)",
			raised: "bg-(--raised)",
		},
		size: {
			bare: "rounded-[3px]",
			s: "rounded-[3px] px-2 py-1.5",
			m: "rounded-[4px] p-3",
			lg: "rounded-[4px] p-[13px]",
		},
	},
	defaultVariants: {
		variant: "sunken",
		size: "m",
	},
});

type PanelVariantProps = VariantProps<typeof panelVariants>;

export type PanelProps = Omit<ComponentProps<typeof Flex.Column>, "children"> &
	PanelVariantProps & {
		children: ReactNode;
	};

/*
Panel is a bordered surface container with sunken or raised backgrounds and
compact, default, or roomy padding.
*/
export const Panel = ({
	variant,
	size,
	className,
	children,
	...props
}: PanelProps) => (
	<Flex.Column
		className={cn(panelVariants({ variant, size }), className)}
		{...props}
	>
		{children}
	</Flex.Column>
);

/*
Title and Caption name the two lines a panel opens with. Unlike Section.Header,
which titles a whole region in a tracked-out uppercase Label, a Panel's title is
mixed case and sits at reading weight: the panel is a card inside a rail, not a
region of the surface, and shouting its name competes with the rail heading
above it.

They exist because this pair was hand-rolled as `font-semibold text-[12px]
text-(--f1)` over `font-mono text-[9.5px] text-(--f4)` in every panel that has
one — the kind of repetition that stays consistent right up until it does not.

Header lays them out with whatever sits opposite the title, which is where a
Badge or a live readout goes.
*/
Panel.Title = ({ ref, className, ...props }: ComponentProps<"span">) => (
	<span
		ref={ref}
		className={cn("font-semibold text-[12px] text-(--f1)", className)}
		{...props}
	/>
);

Panel.Caption = ({ ref, className, ...props }: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn("mt-1 mb-3 font-mono text-[9.5px] text-(--f4)", className)}
		{...props}
	/>
);

export type PanelHeaderProps = Omit<ComponentProps<"div">, "title"> & {
	title?: ReactNode;
	/* The right-hand slot: a Badge, a status word, a painted readout. */
	meta?: ReactNode;
};

Panel.Header = ({
	ref,
	title,
	meta,
	className,
	children,
	...props
}: PanelHeaderProps) => (
	<div
		ref={ref}
		className={cn("flex items-center justify-between", className)}
		{...props}
	>
		{title === undefined ? null : <Panel.Title>{title}</Panel.Title>}
		{children}
		{meta === undefined ? null : meta}
	</div>
);
