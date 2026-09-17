import { cva, type VariantProps } from "class-variance-authority";
import { type HTMLMotionProps, motion } from "motion/react";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Typography } from "./typography";

export const calloutVariants = cva(
	"relative flex min-w-0 flex-col transition-colors",
	{
		variants: {
			tone: {
				accent: [
					"border-[color-mix(in_srgb,var(--acc)_35%,var(--line))]",
					"bg-[color-mix(in_srgb,var(--acc)_6%,var(--sunken))]",
					"[--callout-tone:var(--acc)]",
				],
				info: [
					"border-[color-mix(in_srgb,var(--info)_35%,var(--line))]",
					"bg-[color-mix(in_srgb,var(--info)_6%,var(--sunken))]",
					"[--callout-tone:var(--info)]",
				],
				success: [
					"border-[color-mix(in_srgb,var(--success)_35%,var(--line))]",
					"bg-[color-mix(in_srgb,var(--success)_6%,var(--sunken))]",
					"[--callout-tone:var(--success)]",
				],
				warning: [
					"border-[color-mix(in_srgb,var(--warn)_35%,var(--line))]",
					"bg-[color-mix(in_srgb,var(--warn)_6%,var(--sunken))]",
					"[--callout-tone:var(--warn)]",
				],
				neutral: [
					"border-(--line2)",
					"bg-(--sunken)",
					"[--callout-tone:var(--f2)]",
				],
			},
			size: {
				s: "rounded-[3px] border px-3 py-2.5",
				m: "rounded-[4px] border px-4 py-3",
				lg: "rounded-[6px] border px-5 py-4",
			},
		},
		defaultVariants: {
			tone: "neutral",
			size: "m",
		},
	},
);

type CalloutVariantProps = VariantProps<typeof calloutVariants>;

export type CalloutProps = Omit<HTMLMotionProps<"div">, "children"> &
	CalloutVariantProps & {
		title?: ReactNode;
		meta?: ReactNode;
		children?: ReactNode;
	};

/**
 * Callout is an in-surface informative container panel with subtle tinted backgrounds
 * and hairline borders, used for guidance notes, frozen state notices, or executive summaries.
 */
export const Callout = ({
	title,
	meta,
	tone,
	size,
	className,
	children,
	...props
}: CalloutProps) => {
	return (
		<motion.div
			className={cn(calloutVariants({ tone, size }), className)}
			{...props}
		>
			{(title !== undefined || meta !== undefined) && (
				<div className="flex items-start justify-between gap-4">
					{title !== undefined ? (
						typeof title === "string" ? (
							<Callout.Title>{title}</Callout.Title>
						) : (
							title
						)
					) : null}
					{meta !== undefined && <Callout.Meta>{meta}</Callout.Meta>}
				</div>
			)}
			{children}
		</motion.div>
	);
};

Callout.Title = ({
	className,
	children,
	...props
}: ComponentPropsWithoutRef<typeof Typography.Label>) => (
	<Typography.Label
		size="xs"
		className={cn("text-[color:var(--callout-tone)] font-semibold", className)}
		{...props}
	>
		{children}
	</Typography.Label>
);

Callout.Description = ({
	className,
	children,
	...props
}: ComponentPropsWithoutRef<"p">) => (
	<p
		className={cn("mt-1 text-[10px] text-(--f4) leading-relaxed", className)}
		{...props}
	>
		{children}
	</p>
);

Callout.Meta = ({
	className,
	children,
	...props
}: ComponentPropsWithoutRef<"div">) => (
	<div
		className={cn(
			"shrink-0 text-right font-mono text-[9px] text-(--f4)",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);
