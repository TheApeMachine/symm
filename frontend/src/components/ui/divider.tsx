import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Divider is a rule between regions. It exists so a separator is a thing in the
tree rather than a border bolted onto whichever neighbour happened to be handy —
which is what makes rules double up when panels get reordered.

`weight` picks the line token: `hair` recedes, `strong` reads as a real edge.
*/

export const dividerVariants = cva("shrink-0 border-0", {
	variants: {
		orientation: {
			horizontal: "h-px w-full",
			vertical: "h-full w-px",
		},
		weight: {
			hair: "bg-(--line)",
			strong: "bg-(--line2)",
		},
		inset: {
			none: "",
			s: "",
			m: "",
		},
	},
	compoundVariants: [
		{ orientation: "horizontal", inset: "s", class: "mx-2 w-auto" },
		{ orientation: "horizontal", inset: "m", class: "mx-3 w-auto" },
		{ orientation: "vertical", inset: "s", class: "my-2 h-auto" },
		{ orientation: "vertical", inset: "m", class: "my-3 h-auto" },
	],
	defaultVariants: {
		orientation: "horizontal",
		weight: "hair",
		inset: "none",
	},
});

export type DividerProps = Omit<ComponentProps<"hr">, "children"> &
	VariantProps<typeof dividerVariants>;

export const Divider = ({
	ref,
	orientation,
	weight,
	inset,
	className,
	...props
}: DividerProps) => (
	<hr
		ref={ref}
		aria-orientation={orientation ?? "horizontal"}
		className={cn(dividerVariants({ orientation, weight, inset }), className)}
		{...props}
	/>
);
