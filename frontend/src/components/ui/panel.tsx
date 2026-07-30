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
