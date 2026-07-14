import { cva, type VariantProps } from "class-variance-authority";
import { type ComponentPropsWithoutRef, forwardRef } from "react";
import { cn } from "@/lib/utils";
import type { Size } from "./types";

const BADGE_DOT_SIZE: Record<Size, string> = {
	xxs: "size-1",
	xs: "size-1",
	s: "size-1.5",
	m: "size-1.5",
	lg: "size-2",
	xl: "size-2",
	xxl: "size-2.5",
};

export const badgeVariants = cva(
	[
		"inline-flex items-center gap-1.5 rounded-[2px] border font-semibold uppercase whitespace-nowrap",
		"[--badge-tone:var(--info)]",
		"border-[color:color-mix(in_srgb,var(--badge-tone)_40%,transparent)]",
		"bg-[color:color-mix(in_srgb,var(--badge-tone)_12%,transparent)]",
		"text-[color:var(--badge-tone)]",
	],
	{
		variants: {
			variant: {
				brand: "[--badge-tone:var(--brand)]",
				info: "[--badge-tone:var(--info)]",
				success: "[--badge-tone:var(--success)]",
				warning: "[--badge-tone:var(--warning)]",
				error: "[--badge-tone:var(--error)]",
			},
			size: {
				xxs: "px-1 py-px text-[8px] tracking-[0.06em]",
				xs: "px-[5px] py-0.5 text-[9px] tracking-[0.07em]",
				s: "px-[7px] py-0.5 text-[10px] tracking-[0.08em]",
				m: "px-2 py-0.5 text-[11px] tracking-[0.08em]",
				lg: "px-2.5 py-1 text-xs tracking-[0.08em]",
				xl: "px-3 py-1 text-[13px] tracking-[0.07em]",
				xxl: "px-3 py-1.5 text-sm tracking-[0.06em]",
			},
		},
		defaultVariants: {
			variant: "info",
			size: "s",
		},
	},
);

type BadgeVariantProps = VariantProps<typeof badgeVariants>;

export type BadgeProps = Omit<ComponentPropsWithoutRef<"span">, "children"> &
	BadgeVariantProps & {
		label: string;
		dot?: boolean;
	};

/*
Badge is a compact status pill with semantic color variants and optional dot.
*/
export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(
	({ label, variant, size, dot = false, className, ...props }, ref) => {
		const resolvedSize = (size ?? "s") as Size;

		return (
			<span
				ref={ref}
				className={cn(badgeVariants({ variant, size }), className)}
				{...props}
			>
				{dot ? (
					<span
						className={cn(
							"shrink-0 rounded-full bg-(--badge-tone)",
							BADGE_DOT_SIZE[resolvedSize],
						)}
						aria-hidden
					/>
				) : null}
				{label}
			</span>
		);
	},
);
