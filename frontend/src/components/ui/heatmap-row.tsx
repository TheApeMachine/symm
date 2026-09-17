import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Flex } from "./flex";
import { Meter, type MeterVariant } from "./meter";
import { Typography } from "./typography";

export const heatmapStripVariants = cva("grid flex-1 gap-0.5", {
	variants: {
		columns: {
			8: "grid-cols-8",
			12: "grid-cols-12",
			16: "grid-cols-16",
			24: "grid-cols-24",
			32: "grid-cols-32",
		},
		height: {
			s: "h-6",
			m: "h-10",
			lg: "h-16",
			auto: "h-auto",
		},
	},
	defaultVariants: {
		columns: 16,
		height: "lg",
	},
});

type HeatmapStripVariantProps = VariantProps<typeof heatmapStripVariants>;

export type HeatmapStripProps = ComponentPropsWithoutRef<"div"> &
	HeatmapStripVariantProps & {
		values: number[];
		colorFn?: (value: number) => string;
		cellClassName?: string;
	};

export const HeatmapStrip = ({
	values,
	columns = 16,
	height = "lg",
	colorFn,
	cellClassName,
	className,
	...props
}: HeatmapStripProps) => {
	const occurrences = new Map<string, number>();

	return (
		<div
			className={cn(
				heatmapStripVariants({
					columns: columns as HeatmapStripVariantProps["columns"],
					height,
				}),
				className,
			)}
			{...props}
		>
			{values.map((value) => {
				const keyVal = value.toFixed(4);
				const count = occurrences.get(keyVal) ?? 0;
				occurrences.set(keyVal, count + 1);

				const bg = colorFn ? colorFn(value) : undefined;

				return (
					<div
						key={`${keyVal}-${count}`}
						className={cn("min-w-0 rounded-[1px]", cellClassName)}
						style={bg ? { background: bg } : undefined}
					/>
				);
			})}
		</div>
	);
};

export type HeatmapRowMetricProps = {
	label?: ReactNode;
	value?: ReactNode;
	percent?: number;
	variant?: MeterVariant;
	valueClassName?: string;
	className?: string;
};

export const HeatmapRowMetric = ({
	label = "ε",
	value,
	percent,
	variant = "info",
	valueClassName,
	className,
}: HeatmapRowMetricProps) => {
	return (
		<div className={cn("w-20 shrink-0", className)}>
			<div className="flex justify-between font-mono text-[9px] text-(--f4)">
				<span>{label}</span>
				<span className={valueClassName}>{value}</span>
			</div>
			{percent !== undefined && (
				<Meter
					layout="bar"
					percent={percent}
					variant={variant}
					size="xs"
					className="mt-[3px]"
				/>
			)}
		</div>
	);
};

export type HeatmapRowProps = Omit<
	ComponentPropsWithoutRef<typeof Flex.Row>,
	"children"
> & {
	label: ReactNode;
	labelWidth?: string;
	values?: number[];
	columns?: HeatmapStripVariantProps["columns"];
	stripHeight?: HeatmapStripVariantProps["height"];
	colorFn?: (value: number) => string;
	metric?: ReactNode;
	children?: ReactNode;
};

export const HeatmapRow = ({
	label,
	labelWidth = "w-[92px]",
	values,
	columns = 16,
	stripHeight = "lg",
	colorFn,
	metric,
	children,
	className,
	...props
}: HeatmapRowProps) => {
	return (
		<Flex.Row
			align="center"
			gap={3}
			className={cn("w-full", className)}
			{...props}
		>
			<Typography.Span
				variant="f3"
				className={cn("shrink-0 font-mono text-[10px]", labelWidth)}
			>
				{label}
			</Typography.Span>

			{children ? (
				children
			) : values ? (
				<HeatmapStrip
					values={values}
					columns={columns}
					height={stripHeight}
					colorFn={colorFn}
				/>
			) : null}

			{metric}
		</Flex.Row>
	);
};

HeatmapRow.Strip = HeatmapStrip;
HeatmapRow.Metric = HeatmapRowMetric;

HeatmapRow.Group = ({
	className,
	children,
	...props
}: ComponentPropsWithoutRef<typeof Flex.Column>) => {
	return (
		<Flex.Column gap={2} className={cn("w-full", className)} {...props}>
			{children}
		</Flex.Column>
	);
};
