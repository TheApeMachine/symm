import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Flex } from "./flex";

export const dataRowVariants = cva("min-w-0 transition-colors", {
	variants: {
		layout: {
			inline: "flex items-baseline justify-between gap-3",
			detailed: "flex flex-col gap-0.5",
			stacked: "flex flex-col gap-1",
		},
		density: {
			compact: "py-1 px-2.5",
			normal: "py-2 px-3",
			spacious: "py-3 px-3.5",
			bare: "",
		},
		border: {
			none: "",
			bottom: "border-(--line) border-b last:border-b-0",
		},
	},
	defaultVariants: {
		layout: "inline",
		density: "normal",
		border: "none",
	},
});

export const dataRowValueVariants = cva("text-right font-mono text-[10px]", {
	variants: {
		tone: {
			default: "text-(--f1)",
			accent: "text-(--acc)",
			up: "text-(--up)",
			down: "text-(--down)",
			warning: "text-(--warn)",
			info: "text-(--info)",
			f1: "text-(--f1)",
			f2: "text-(--f2)",
			f3: "text-(--f3)",
			f4: "text-(--f4)",
		},
		size: {
			xs: "text-[9px]",
			s: "text-[10px]",
			m: "text-[11px]",
			lg: "text-sm font-semibold",
		},
	},
	defaultVariants: {
		tone: "default",
		size: "s",
	},
});

type DataRowVariantProps = VariantProps<typeof dataRowVariants>;
type DataRowValueVariantProps = VariantProps<typeof dataRowValueVariants>;

export type DataRowProps = Omit<ComponentPropsWithoutRef<"div">, "children"> &
	DataRowVariantProps &
	DataRowValueVariantProps & {
		label: ReactNode;
		value: ReactNode;
		help?: ReactNode;
		description?: ReactNode;
		paintKey?: string;
		labelClassName?: string;
		valueClassName?: string;
	};

/**
 * DataRow displays a labeled key-value pair with inline, detailed, or stacked layouts,
 * semantic color tones, and direct DOM painting slots.
 */
export const DataRow = ({
	label,
	value,
	help,
	description,
	paintKey,
	layout,
	density,
	border,
	tone,
	size,
	className,
	labelClassName,
	valueClassName,
	...props
}: DataRowProps) => {
	const resolvedHelp = help ?? description;
	const resolvedLayout = layout ?? (resolvedHelp ? "detailed" : "inline");

	// Value slot: wraps or forwards paintKey attribute for direct DOM painting
	const renderedValue = (
		<span
			data-f={paintKey}
			className={cn(dataRowValueVariants({ tone, size }), valueClassName)}
		>
			{value}
		</span>
	);

	if (resolvedLayout === "detailed") {
		return (
			<div
				className={cn(
					dataRowVariants({ layout: "detailed", density, border }),
					className,
				)}
				{...props}
			>
				<div className="flex items-baseline justify-between gap-3">
					<span
						className={cn(
							"font-mono text-[9px] text-(--f3) select-none",
							labelClassName,
						)}
					>
						{label}
					</span>
					{renderedValue}
				</div>
				{resolvedHelp && (
					<p className="text-[9px] text-(--f4) leading-relaxed">
						{resolvedHelp}
					</p>
				)}
			</div>
		);
	}

	if (resolvedLayout === "stacked") {
		return (
			<div
				className={cn(
					dataRowVariants({ layout: "stacked", density, border }),
					className,
				)}
				{...props}
			>
				<span
					className={cn(
						"font-mono text-[9px] text-(--f3) select-none",
						labelClassName,
					)}
				>
					{label}
				</span>
				<div className="text-left">{renderedValue}</div>
				{resolvedHelp && (
					<p className="text-[9px] text-(--f4) leading-relaxed">
						{resolvedHelp}
					</p>
				)}
			</div>
		);
	}

	return (
		<div
			className={cn(
				dataRowVariants({ layout: "inline", density, border }),
				className,
			)}
			{...props}
		>
			<span
				className={cn(
					"font-mono text-[9px] text-(--f3) select-none",
					labelClassName,
				)}
			>
				{label}
			</span>
			{renderedValue}
		</div>
	);
};

export type DataRowGroupProps = ComponentPropsWithoutRef<typeof Flex.Column> & {
	density?: DataRowVariantProps["density"];
	border?: boolean;
	children: ReactNode;
};

/**
 * DataRow.Group wraps multiple DataRows, applying hairline borders between items.
 */
DataRow.Group = ({
	density,
	border = true,
	className,
	children,
	...props
}: DataRowGroupProps) => {
	return (
		<Flex.Column
			className={cn(
				"w-full",
				border && "*:border-(--line) *:border-b last:*:border-b-0",
				className,
			)}
			{...props}
		>
			{children}
		</Flex.Column>
	);
};
