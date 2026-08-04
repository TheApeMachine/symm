import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon } from "./icon";
import type { Size } from "./types";

/*
Input is a text field with the chrome taken off.

Everything in this UI that accepts typing sits inside something that already has
an edge — a header strip with a bottom rule, a rounded search box. A field that
drew its own border would double that edge, so Input draws none: no background,
no outline, no ring. The box, when there is one, is Field's job.
*/

export const inputVariants = cva(
	[
		"min-w-0 flex-1 bg-transparent text-(--f1) outline-none",
		"placeholder:text-(--f4)",
		"disabled:cursor-not-allowed disabled:opacity-50",
	],
	{
		variants: {
			size: {
				xxs: "h-5 text-[10px]",
				xs: "h-6 text-[10.5px]",
				s: "h-7 text-[11px]",
				m: "h-8 text-[12px]",
				lg: "h-9 text-[13px]",
				xl: "h-10 text-[15px]",
				xxl: "h-11 text-base",
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

export type InputProps = Omit<ComponentProps<"input">, "size"> &
	VariantProps<typeof inputVariants>;

export const Input = ({
	ref,
	size,
	mono,
	className,
	spellCheck = false,
	...props
}: InputProps) => (
	<input
		ref={ref}
		spellCheck={spellCheck}
		className={cn(inputVariants({ size, mono }), className)}
		{...props}
	/>
);

/*
Field is the box a control sits in: a leading glyph, the control, and a trailing
slot for whatever annotates it — a result count, a shortcut hint, a unit.

`bare` is for a field whose container already supplies the edge; `box` draws its
own. Both keep the same internal rhythm, so a palette input and a sidebar search
line up even though only one of them has a border.
*/

export const fieldVariants = cva("flex items-center", {
	variants: {
		variant: {
			box: "rounded-md border border-(--line) bg-(--surface) px-2",
			bare: "",
		},
		gap: {
			s: "gap-1.5",
			m: "gap-2.5",
		},
	},
	defaultVariants: {
		variant: "box",
		gap: "s",
	},
});

export type FieldProps = ComponentProps<"div"> &
	VariantProps<typeof fieldVariants> & {
		leading?: ReactNode;
		trailing?: ReactNode;
	};

Input.Field = ({
	ref,
	variant,
	gap,
	leading,
	trailing,
	className,
	children,
	...props
}: FieldProps) => (
	<div
		ref={ref}
		className={cn(fieldVariants({ variant, gap }), className)}
		{...props}
	>
		{leading}
		{children}
		{trailing}
	</div>
);

export type SearchProps = InputProps & {
	variant?: VariantProps<typeof fieldVariants>["variant"];
	/*
		Styles the box. `className` still reaches the input itself, so the common
		case — restyling what you type in — needs no extra prop.
	*/
	fieldClassName?: string;
	trailing?: ReactNode;
	/*
		The glyph does not track the field's size. A field grows to make its text
		comfortable to read; past a point the magnifier beside it just gets heavy.
	*/
	iconSize?: Size;
};

/*
Search is the shape this app reaches for by default: magnifier, field, optional
annotation. The ref lands on the input, because the only reason a caller holds
one is to focus it.
*/
Input.Search = ({
	ref,
	variant,
	fieldClassName,
	trailing,
	iconSize = "s",
	size,
	mono,
	className,
	...props
}: SearchProps) => (
	<Input.Field
		variant={variant}
		gap={variant === "bare" ? "m" : "s"}
		className={fieldClassName}
		leading={<Icon name="search" size={iconSize} className="text-(--f4)" />}
		trailing={trailing}
	>
		<Input ref={ref} size={size} mono={mono} className={className} {...props} />
	</Input.Field>
);
