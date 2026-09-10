import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Textarea is Input's multi-line sibling, and takes its chrome off for the same
reason: whatever holds it already has an edge.
*/

export const textareaVariants = cva(
	[
		"min-w-0 flex-1 resize-none bg-transparent text-(--f1) outline-none",
		"placeholder:text-(--f4)",
		"disabled:cursor-not-allowed disabled:opacity-50",
	],
	{
		variants: {
			size: {
				xs: "text-[10.5px]",
				s: "text-[11px]",
				m: "text-[12px]",
				lg: "text-[13px]",
			},
			mono: {
				true: "font-mono",
			},
		},
		defaultVariants: {
			size: "s",
		},
	},
);

export type TextareaProps = Omit<ComponentProps<"textarea">, "size"> &
	VariantProps<typeof textareaVariants>;

export const Textarea = ({
	ref,
	size,
	mono,
	className,
	...props
}: TextareaProps) => (
	<textarea
		ref={ref}
		className={cn(textareaVariants({ size, mono }), className)}
		{...props}
	/>
);
