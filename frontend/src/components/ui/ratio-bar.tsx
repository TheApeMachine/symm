import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
RatioBar is a proportional segmented meter displaying the relative weights
of multiple quantities (e.g. positive vs negative outcomes, sympathy clusters).

Each segment occupies its share of the total length using semantic tone variables.
*/

export const ratioBarTrackVariants = cva(
	"flex w-full overflow-hidden rounded-full bg-(--line)",
	{
		variants: {
			size: {
				xs: "h-1",
				s: "h-1.5",
				m: "h-2",
				lg: "h-3",
			},
		},
		defaultVariants: {
			size: "s",
		},
	},
);

const TONE_BG_CLASS: Record<string, string> = {
	up: "bg-(--up)",
	down: "bg-(--down)",
	acc: "bg-(--acc)",
	brand: "bg-(--brand)",
	info: "bg-(--info)",
	warn: "bg-(--warn)",
	error: "bg-(--error)",
	f1: "bg-(--f1)",
	f2: "bg-(--f2)",
	f3: "bg-(--f3)",
	f4: "bg-(--f4)",
};

export type RatioBarSegment = {
	value: number;
	tone?: keyof typeof TONE_BG_CLASS;
	label?: string;
};

export type RatioBarProps = ComponentProps<"div"> &
	VariantProps<typeof ratioBarTrackVariants> & {
		segments: RatioBarSegment[];
	};

export const RatioBar = ({
	ref,
	segments,
	size,
	className,
	...props
}: RatioBarProps) => {
	const total = segments.reduce((sum, s) => sum + Math.max(0, s.value), 0);

	return (
		<div
			ref={ref}
			role="progressbar"
			aria-valuemin={0}
			aria-valuemax={100}
			className={cn(ratioBarTrackVariants({ size }), className)}
			{...props}
		>
			{total > 0
				? segments.map((seg) => {
						if (seg.value <= 0) return null;
						const pct = (seg.value / total) * 100;
						const bgClass = TONE_BG_CLASS[seg.tone ?? "acc"] ?? "bg-(--acc)";

						return (
							<div
								key={seg.label ?? seg.tone ?? "acc"}
								title={seg.label ? `${seg.label}: ${seg.value}` : undefined}
								style={{ width: `${pct}%` }}
								className={cn(
									"h-full transition-[width] duration-300",
									bgClass,
								)}
							/>
						);
					})
				: null}
		</div>
	);
};
