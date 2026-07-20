import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";
import type { Size } from "./types";

export const statVariants = cva("", {
	variants: {
		layout: {
			metric: "",
			tile: "rounded-[3px] border border-(--line) bg-(--surface) px-2 py-1.5",
			feature: "px-1 py-1",
		},
	},
	defaultVariants: {
		layout: "metric",
	},
});

const statValueVariants = cva(
	"font-mono [--stat-tone:var(--f1)] text-[color:var(--stat-tone)]",
	{
		variants: {
			variant: {
				brand: "[--stat-tone:var(--brand)]",
				info: "[--stat-tone:var(--info)]",
				success: "[--stat-tone:var(--success)]",
				warning: "[--stat-tone:var(--warning)]",
				error: "[--stat-tone:var(--error)]",
				disabled: "[--stat-tone:var(--f3)]",
			},
			emphasis: {
				default: "",
				strong: "font-semibold",
			},
			size: {
				xxs: "text-[10px] leading-none",
				xs: "text-[10px] leading-none",
				s: "text-[11px] leading-none",
				m: "text-[19px] leading-none",
				lg: "text-2xl font-semibold leading-none",
				xl: "text-2xl font-semibold leading-none",
				xxl: "text-3xl font-semibold leading-none",
			},
		},
		defaultVariants: {
			emphasis: "default",
			size: "lg",
		},
	},
);

const statLabelVariants = cva("font-mono text-(--f4)", {
	variants: {
		layout: {
			metric: "mt-1 text-[9px]",
			tile: "text-[8.5px] uppercase tracking-[0.08em]",
			feature: "text-[8.5px] uppercase tracking-[0.08em]",
		},
	},
	defaultVariants: {
		layout: "metric",
	},
});

const VALUE_SIZE_BY_LAYOUT: Record<
	NonNullable<StatLayoutProps["layout"]>,
	Size
> = {
	metric: "lg",
	tile: "s",
	feature: "m",
};

const VALUE_SPACING_BY_LAYOUT: Record<
	NonNullable<StatLayoutProps["layout"]>,
	string
> = {
	metric: "",
	tile: "mt-0.5",
	feature: "mt-1",
};

type StatLayoutProps = VariantProps<typeof statVariants>;
type StatValueProps = VariantProps<typeof statValueVariants>;

export type StatProps = Omit<ComponentProps<"div">, "children"> &
	StatLayoutProps &
	Pick<StatValueProps, "variant" | "size" | "emphasis"> & {
		value: string;
		label: string;
		valueClassName?: string;
		labelClassName?: string;
	};

/*
Stat displays a labeled metric with metric, tile, or feature layouts and
semantic value tones.
*/
export const Stat = ({
	ref,
	value,
	label,
	layout,
	variant,
	size,
	emphasis,
	valueClassName,
	labelClassName,
	className,
	...props
}: StatProps) => {
	const resolvedLayout = layout ?? "metric";
	const resolvedSize = (size ?? VALUE_SIZE_BY_LAYOUT[resolvedLayout]) as Size;
	const labelFirst = resolvedLayout === "tile" || resolvedLayout === "feature";

	const valueNode = (
		<div
			data-stat-value="true"
			className={cn(
				statValueVariants({ variant, size: resolvedSize, emphasis }),
				VALUE_SPACING_BY_LAYOUT[resolvedLayout],
				valueClassName,
			)}
		>
			{value}
		</div>
	);

	const labelNode = (
		<div
			className={cn(
				statLabelVariants({ layout: resolvedLayout }),
				labelClassName,
			)}
		>
			{label}
		</div>
	);

	return (
		<div
			ref={ref}
			className={cn(statVariants({ layout }), className)}
			{...props}
		>
			{labelFirst ? labelNode : valueNode}
			{labelFirst ? valueNode : labelNode}
		</div>
	);
};
