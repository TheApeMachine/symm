import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

/*
Button separates what a control is made of from what it means.

`variant` is chrome — how much surface the control occupies. `tone` is meaning,
and it only ever sets --button-tone; every variant reads that same property, so
a tone works the same whether it lands on a filled button or a bare one. Getting
these onto one axis is what made the old set contradict itself.

`bare` exists because a large share of this UI's controls are clickable regions
that must not look like buttons at all — a badge you can press, a row you can
select. Those still need the button element for keyboard and assistive-tech
behaviour; they just want none of its paint.
*/

export const buttonVariants = cva(
	[
		"inline-flex cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap pointer-events-auto",
		"[--button-tone:var(--f2)]",
		"transition-colors",
		"disabled:pointer-events-none disabled:opacity-45",
		"focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-(--acc)",
	],
	{
		variants: {
			tone: {
				brand: "[--button-tone:var(--brand)]",
				accent: "[--button-tone:var(--acc)]",
				info: "[--button-tone:var(--info)]",
				success: "[--button-tone:var(--success)]",
				warning: "[--button-tone:var(--warning)]",
				error: "[--button-tone:var(--error)]",
				muted: "[--button-tone:var(--f3)]",
				disabled: "[--button-tone:var(--f4)]",
			},
			variant: {
				solid: [
					"rounded-[3px] border font-semibold",
					"border-[color:color-mix(in_srgb,var(--button-tone)_45%,transparent)]",
					"bg-[color:color-mix(in_srgb,var(--button-tone)_16%,transparent)]",
					"text-[color:var(--button-tone)]",
					"hover:bg-[color:color-mix(in_srgb,var(--button-tone)_26%,transparent)]",
				],
				outline: [
					"rounded-[3px] border border-(--line) bg-(--raised) text-(--f3)",
					"hover:border-(--line2) hover:text-(--f1)",
				],
				quiet: [
					"rounded-[3px] border border-transparent bg-transparent text-(--f3)",
					"hover:bg-(--raised) hover:text-(--f1)",
				],
				bare: "border-0 bg-transparent p-0 text-left font-[inherit] text-[color:inherit]",
			},
			size: {
				xxs: "px-1 py-px text-[9px]",
				xs: "px-1.5 py-0.5 text-[10px]",
				s: "px-2 py-1 text-[11px]",
				m: "px-2.5 py-1.25 text-[12px]",
				lg: "px-3 py-1.5 text-[13px]",
				xl: "px-3.5 py-2 text-sm",
				xxl: "px-4 py-2.5 text-base",
			},
			shape: {
				default: "",
				/* A square control whose glyph is its whole content. */
				icon: "aspect-square p-0",
				block: "w-full",
			},
		},
		/*
			`bare` means "no chrome", and padding is chrome. Letting size through
			would put a solid button's box back on a control that asked for none.
		*/
		compoundVariants: [
			{ variant: "bare", class: "p-0" },
			{ variant: "quiet", shape: "block", class: "justify-start" },
			/* An icon control is a box, not a line of text, so size means edge. */
			{ shape: "icon", size: "xxs", class: "size-4" },
			{ shape: "icon", size: "xs", class: "size-5" },
			{ shape: "icon", size: "s", class: "size-[25px]" },
			{ shape: "icon", size: "m", class: "size-7" },
			{ shape: "icon", size: "lg", class: "size-8" },
			{ shape: "icon", size: "xl", class: "size-9" },
			{ shape: "icon", size: "xxl", class: "size-10" },
		],
		defaultVariants: {
			variant: "quiet",
			size: "s",
			shape: "default",
		},
	},
);

type ButtonVariantProps = VariantProps<typeof buttonVariants>;

export type ButtonProps = Omit<ComponentProps<"button">, "children"> &
	ButtonVariantProps & {
		children?: ReactNode;
	};

export const Button = ({
	ref,
	variant,
	tone,
	size,
	shape,
	type = "button",
	className,
	children,
	...props
}: ButtonProps) => (
	<button
		ref={ref}
		type={type}
		className={cn(buttonVariants({ variant, tone, size, shape }), className)}
		{...props}
	>
		{children}
	</button>
);
