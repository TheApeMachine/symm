import { cva, type VariantProps } from "class-variance-authority";
import { type ComponentPropsWithoutRef, forwardRef } from "react";
import { cn } from "@/lib/utils";
import type { Size } from "./types";

const clampPercent = (percent: number) => Math.max(0, Math.min(100, percent));

export const meterTrackVariants = cva(
	"overflow-hidden bg-(--line) [--meter-tone:var(--info)]",
	{
		variants: {
			variant: {
				brand: "[--meter-tone:var(--brand)]",
				info: "[--meter-tone:var(--info)]",
				success: "[--meter-tone:var(--success)]",
				warning: "[--meter-tone:var(--warning)]",
				error: "[--meter-tone:var(--error)]",
			},
			size: {
				xxs: "h-1 rounded-[2px]",
				xs: "h-1 rounded-[2px]",
				s: "h-[5px] rounded-[3px]",
				m: "h-1.5 rounded-[3px]",
				lg: "h-2 rounded-sm",
				xl: "h-2.5 rounded",
				xxl: "h-3 rounded",
			},
		},
		defaultVariants: {
			variant: "info",
			size: "s",
		},
	},
);

export const meterVariants = cva("", {
	variants: {
		layout: {
			stacked: "",
			inline: "flex items-center gap-2",
			bar: "",
		},
	},
	defaultVariants: {
		layout: "stacked",
	},
});

const meterHeaderVariants = cva("mb-1 flex justify-between font-mono", {
	variants: {
		size: {
			xxs: "text-[9px]",
			xs: "text-[9px]",
			s: "text-[10px]",
			m: "text-[10.5px]",
			lg: "text-[11px]",
			xl: "text-[11px]",
			xxl: "text-xs",
		},
	},
	defaultVariants: {
		size: "s",
	},
});

export const meterInlineLabelClass: Record<Size, string> = {
	xxs: "w-[50px] shrink-0 text-[9px] text-(--f4)",
	xs: "w-[54px] shrink-0 text-[9px] text-(--f4)",
	s: "w-[58px] shrink-0 text-[10px] text-(--f4)",
	m: "w-[38px] shrink-0 font-mono text-[10px] text-(--f3)",
	lg: "w-10 shrink-0 font-mono text-[11px] text-(--f3)",
	xl: "w-11 shrink-0 font-mono text-[11px] text-(--f3)",
	xxl: "w-12 shrink-0 font-mono text-xs text-(--f3)",
};

export const meterInlineValueClass: Record<Size, string> = {
	xxs: "w-4 shrink-0 text-right font-mono text-[9px] text-(--f2)",
	xs: "w-[18px] shrink-0 text-right font-mono text-[9px] text-(--f2)",
	s: "w-[18px] shrink-0 text-right font-mono text-[10px] text-(--f2)",
	m: "w-14 shrink-0 text-right font-mono text-[9px] text-(--f4)",
	lg: "w-14 shrink-0 text-right font-mono text-[10px] text-(--f4)",
	xl: "w-16 shrink-0 text-right font-mono text-[10.5px] text-(--f4)",
	xxl: "w-[4.5rem] shrink-0 text-right font-mono text-[11px] text-(--f4)",
};

type MeterTrackVariantProps = VariantProps<typeof meterTrackVariants>;
type MeterLayoutProps = VariantProps<typeof meterVariants>;

export type MeterProps = Omit<ComponentPropsWithoutRef<"div">, "children"> &
	MeterTrackVariantProps &
	MeterLayoutProps & {
		percent: number;
		label?: string;
		value?: string;
		animated?: boolean;
		labelClassName?: string;
		valueClassName?: string;
		trackClassName?: string;
	};

/*
Meter is a labeled progress bar with semantic fill variants and stacked, inline,
or bar-only layouts.
*/
export const Meter = forwardRef<HTMLDivElement, MeterProps>(
	(
		{
			percent,
			label,
			value,
			variant,
			size,
			layout,
			animated = false,
			labelClassName,
			valueClassName,
			trackClassName,
			className,
			...props
		},
		ref,
	) => {
		const resolvedSize = (size ?? "s") as Size;
		const resolvedLayout = layout ?? "stacked";
		const fillWidth = clampPercent(percent);

		return (
			<div
				ref={ref}
				className={cn(meterVariants({ layout }), className)}
				{...props}
				role="progressbar"
				aria-valuenow={fillWidth}
				aria-valuemin={0}
				aria-valuemax={100}
			>
				{resolvedLayout === "stacked" &&
				(label !== undefined || value !== undefined) ? (
					<div className={meterHeaderVariants({ size: resolvedSize })}>
						<span className={cn("text-(--f3)", labelClassName)}>{label}</span>
						<span className={cn("text-(--f1)", valueClassName)}>{value}</span>
					</div>
				) : null}

				{resolvedLayout === "inline" ? (
					<>
						{label === undefined ? null : (
							<span
								data-inline-label="true"
								className={cn(
									meterInlineLabelClass[resolvedSize],
									labelClassName,
								)}
							>
								{label}
							</span>
						)}
						<div
							data-inline-track="true"
							className={cn(
								meterTrackVariants({ variant, size: resolvedSize }),
								"flex-1",
								trackClassName,
							)}
						>
							<div
								data-inline-fill="true"
								className={cn(
									"h-full bg-(--meter-tone)",
									animated && "transition-[width] duration-500 ease-out",
								)}
								style={{ width: `${fillWidth}%` }}
							/>
						</div>
						{value === undefined ? null : (
							<span
								data-inline-value="true"
								className={cn(
									meterInlineValueClass[resolvedSize],
									valueClassName,
								)}
							>
								{value}
							</span>
						)}
					</>
				) : (
					<div
						className={cn(
							meterTrackVariants({ variant, size: resolvedSize }),
							resolvedLayout === "bar" && "w-full",
							trackClassName,
						)}
					>
						<div
							className={cn(
								"h-full bg-(--meter-tone)",
								animated && "transition-[width] duration-500 ease-out",
							)}
							style={{ width: `${fillWidth}%` }}
						/>
					</div>
				)}
			</div>
		);
	},
);
