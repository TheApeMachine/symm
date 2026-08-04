import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

/*
Chip is a mono `label value` pill for readouts that are facts about the view
rather than states of a thing — node counts, edge counts, the frame's timestamp.

It is not a Badge. A Badge says what something *is* and carries a semantic tone;
a Chip says what something *measures* and stays neutral, which is why it has a
value slot and no variant scale. Reaching for a Badge here is what made status
colours mean nothing in the header strip.

Both label and value take nodes, so either half can be a painted span.
*/

export const chipVariants = cva(
	"inline-flex items-center gap-1 whitespace-nowrap font-mono text-(--f3)",
	{
		variants: {
			variant: {
				outline: "rounded-[3px] border border-(--line) bg-(--raised)",
				quiet: "rounded-[3px] border border-(--line)",
				bare: "",
			},
			size: {
				xxs: "px-1 py-px text-[8.5px]",
				xs: "px-1.5 py-0.5 text-[9.5px]",
				s: "px-1.5 py-0.5 text-[10px]",
				m: "px-2 py-1 text-[10px]",
				lg: "px-2.5 py-1 text-[11px]",
				xl: "px-3 py-1.5 text-xs",
				xxl: "px-3.5 py-2 text-sm",
			},
		},
		compoundVariants: [{ variant: "bare", class: "p-0" }],
		defaultVariants: {
			variant: "outline",
			size: "m",
		},
	},
);

type ChipVariantProps = VariantProps<typeof chipVariants>;

export type ChipProps = Omit<ComponentProps<"div">, "children"> &
	ChipVariantProps & {
		label: ReactNode;
		value?: ReactNode;
		children?: ReactNode;
	};

export const Chip = ({
	ref,
	label,
	value,
	variant,
	size,
	className,
	children,
	...props
}: ChipProps) => (
	<div
		ref={ref}
		className={cn(chipVariants({ variant, size }), className)}
		{...props}
	>
		<span className="text-(--f4)">{label}</span>
		{value === undefined ? null : <span>{value}</span>}
		{children}
	</div>
);
