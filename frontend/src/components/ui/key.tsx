import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

/*
Key renders a keyboard cap.

The modifier is a prop rather than baked into the glyph, because ⌘ is wrong on
every platform but one and a shortcut hint that lies is worse than no hint at
all. The default stays "cmd" since that is what this UI's own bindings use, but
a host app can pass whatever its platform actually wants.

It renders <kbd> rather than <span>: a shortcut hint is one of the few things
HTML already has an element for.
*/

const MODIFIER_GLYPH = {
	cmd: "⌘",
	ctrl: "⌃",
	alt: "⌥",
	shift: "⇧",
	none: "",
} as const;

export type Modifier = keyof typeof MODIFIER_GLYPH;

export const keyVariants = cva(
	"inline-flex items-center rounded-xs border border-(--line) font-mono text-(--f4)",
	{
		variants: {
			size: {
				xs: "px-0.5 text-[9px]",
				s: "px-1 text-[10px]",
				m: "px-1.5 py-0.5 text-[11px]",
			},
		},
		defaultVariants: {
			size: "s",
		},
	},
);

export type KeyProps = Omit<ComponentProps<"kbd">, "children"> &
	VariantProps<typeof keyVariants> & {
		modifier?: Modifier;
		/* The key itself; a single letter is capitalised the way caps are. */
		children?: ReactNode;
	};

export const Key = ({
	ref,
	modifier = "cmd",
	size,
	className,
	children,
	...props
}: KeyProps) => (
	<kbd ref={ref} className={cn(keyVariants({ size }), className)} {...props}>
		{MODIFIER_GLYPH[modifier]}
		{typeof children === "string" ? children.toUpperCase() : children}
	</kbd>
);
