import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { Size } from "./types";

/*
Dot is a state indicator sized to sit inline with text.

It is deliberately a plain <span> that forwards everything: a readiness gate
paints it through data-set + data-target="style.background", and a kernel row
swaps its fill through data-paint-class. Neither can work if the primitive
re-renders or swallows attributes, so Dot holds no state of its own.

Children are allowed and hidden from sight but not from assistive tech — that is
how a painted dot keeps a readable fallback ("STANDBY") in the DOM while the
engine has said nothing yet.
*/

export const dotVariants = cva("shrink-0 rounded-full [--dot-tone:var(--f3)]", {
	variants: {
		variant: {
			brand: "[--dot-tone:var(--brand)]",
			accent: "[--dot-tone:var(--acc)]",
			info: "[--dot-tone:var(--info)]",
			success: "[--dot-tone:var(--success)]",
			warning: "[--dot-tone:var(--warning)]",
			error: "[--dot-tone:var(--error)]",
			muted: "[--dot-tone:var(--f3)]",
			disabled: "[--dot-tone:var(--line2)]",
		},
		size: {
			xxs: "size-1",
			xs: "size-1",
			s: "size-1.5",
			m: "size-1.5",
			lg: "size-2",
			xl: "size-2",
			xxl: "size-2.5",
		},
		fill: {
			solid: "bg-(--dot-tone)",
			hollow: "border border-(--dot-tone) bg-transparent",
			halo: "bg-(--dot-tone) ring-2 ring-[color:color-mix(in_srgb,var(--dot-tone)_28%,transparent)]",
		},
		pulse: {
			true: "motion-safe:animate-pulse",
		},
	},
	defaultVariants: {
		variant: "muted",
		size: "s",
		fill: "solid",
	},
});

type DotVariantProps = VariantProps<typeof dotVariants>;

export type DotProps = Omit<ComponentProps<"span">, "children"> &
	DotVariantProps & {
		/* Sight-hidden fallback text, kept readable for assistive tech. */
		children?: ReactNode;
	};

export const Dot = ({
	ref,
	variant,
	size,
	fill,
	pulse,
	className,
	children,
	...props
}: DotProps) => (
	<span
		ref={ref}
		/*
			A dot with no fallback text says nothing a screen reader can use, so it
			stays out of the tree. One that carries text is that text's only host.
		*/
		aria-hidden={children === undefined ? true : undefined}
		className={cn(
			dotVariants({ variant, size, fill, pulse }),
			children === undefined
				? undefined
				: "overflow-hidden text-[0] leading-none",
			className,
		)}
		{...props}
	>
		{children}
	</span>
);

/*
DOT_SIZE_FOR is how a container picks the dot that belongs inside it: one step
down from its own size, so a badge never looks like it is wearing a bead.
*/
export const DOT_SIZE_FOR: Record<Size, Size> = {
	xxs: "xxs",
	xs: "xxs",
	s: "xs",
	m: "s",
	lg: "s",
	xl: "m",
	xxl: "lg",
};
