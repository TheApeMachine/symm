import { cva, type VariantProps } from "class-variance-authority";
import { type HTMLMotionProps, motion } from "motion/react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Typography } from "./typography";

export const stepCardVariants = cva(
	"relative flex min-w-0 flex-col rounded-[4px] border p-3 transition-colors",
	{
		variants: {
			variant: {
				sunken: "border-(--line) bg-(--sunken)",
				surface: "border-(--line) bg-(--surface)",
				raised: "border-(--line2) bg-(--raised)",
			},
			status: {
				default: "[--step-tone:var(--acc)]",
				success: "[--step-tone:var(--success)]",
				warning: "[--step-tone:var(--warn)]",
				error: "[--step-tone:var(--error)]",
				info: "[--step-tone:var(--info)]",
			},
		},
		defaultVariants: {
			variant: "sunken",
			status: "default",
		},
	},
);

type StepCardVariantProps = VariantProps<typeof stepCardVariants>;

export type StepCardProps = Omit<HTMLMotionProps<"div">, "children"> &
	StepCardVariantProps & {
		step: ReactNode;
		title: ReactNode;
		value: ReactNode;
		description?: ReactNode;
		explanation?: ReactNode;
		footer?: ReactNode;
	};

/**
 * StepCard displays a sequential stage or decision snapshot with an index badge,
 * title, mono value readout, and explanatory subtext.
 */
export const StepCard = ({
	step,
	title,
	value,
	description,
	explanation,
	footer,
	variant,
	status,
	className,
	...props
}: StepCardProps) => {
	const resolvedText = description ?? explanation;

	return (
		<motion.div
			className={cn(stepCardVariants({ variant, status }), className)}
			whileHover={{ borderColor: "var(--line2)" }}
			transition={{ duration: 0.15 }}
			{...props}
		>
			<div className="mb-2 flex items-center gap-2">
				<span className="flex size-5 shrink-0 items-center justify-center rounded-full border border-(--line2) font-mono text-[9px] text-[color:var(--step-tone)] select-none">
					{step}
				</span>
				<Typography.Label size="s" tone="f2" className="truncate">
					{title}
				</Typography.Label>
			</div>
			<div className="font-mono text-[12px] text-(--f1) leading-snug">
				{value}
			</div>
			{resolvedText && (
				<p className="mt-1.5 text-[10px] text-(--f4) leading-relaxed">
					{resolvedText}
				</p>
			)}
			{footer && <div className="mt-2 pt-2 border-t border-(--line)">{footer}</div>}
		</motion.div>
	);
};
