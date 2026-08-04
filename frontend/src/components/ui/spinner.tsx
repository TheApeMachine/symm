import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Spinner is a "sampling sweep" — a row of thin probe bars with a lit amber
cursor traveling across them, leaving a fading trail. It reads as measuring /
scanning rather than a generic rotating ring, matching the signal-kernel
sparkline + scanline motif of the terminal. Purely decorative motion; label it
for assistive tech via the surrounding "waiting" copy.

The sweep keyframes live in theme.css alongside the tokens, so a page with a
dozen spinners on it ships one copy of them instead of a dozen inline <style>
blocks. Respects reduced-motion by falling back to a static dimmed row.
*/

const SPINNER_BARS = 5;

const spinnerVariants = cva("inline-flex items-end [--spin-tone:var(--acc)]", {
	variants: {
		variant: {
			brand: "[--spin-tone:var(--brand)]",
			accent: "[--spin-tone:var(--acc)]",
			info: "[--spin-tone:var(--info)]",
			success: "[--spin-tone:var(--success)]",
			warning: "[--spin-tone:var(--warning)]",
			error: "[--spin-tone:var(--error)]",
			muted: "[--spin-tone:var(--f3)]",
		},
		size: {
			xxs: "gap-[2px] [--spin-h:8px] [--spin-w:1px]",
			xs: "gap-[2px] [--spin-h:10px] [--spin-w:1px]",
			s: "gap-[3px] [--spin-h:12px] [--spin-w:1.5px]",
			m: "gap-[3px] [--spin-h:14px] [--spin-w:1.5px]",
			lg: "gap-1 [--spin-h:18px] [--spin-w:2px]",
			xl: "gap-1 [--spin-h:22px] [--spin-w:2px]",
			xxl: "gap-1.5 [--spin-h:28px] [--spin-w:2.5px]",
		},
	},
	defaultVariants: {
		variant: "accent",
		size: "s",
	},
});

type SpinnerVariantProps = VariantProps<typeof spinnerVariants>;

export type SpinnerProps = Omit<ComponentProps<"output">, "children"> &
	SpinnerVariantProps & {
		/** Total sweep duration in ms (lower = faster). */
		durationMs?: number;
		/** Accessible label announced to screen readers. */
		label?: string;
	};

export const Spinner = ({
	ref,
	variant,
	size,
	durationMs = 1100,
	label = "Loading",
	className,
	style,
	...props
}: SpinnerProps) => {
	return (
		<output
			ref={ref}
			aria-label={label}
			className={cn(spinnerVariants({ variant, size }), className)}
			style={
				{
					...style,
					"--spin-dur": `${durationMs}ms`,
				} as React.CSSProperties
			}
			{...props}
		>
			{Array.from({ length: SPINNER_BARS }, (_, i) => (
				<span
					// biome-ignore lint/suspicious/noArrayIndexKey: fixed-length static bars
					key={i}
					className={cn(
						"w-(--spin-w) origin-bottom rounded-[0.5px] bg-(--spin-tone)",
						"motion-safe:animate-[symSweep_var(--spin-dur)_linear_infinite]",
						"motion-reduce:opacity-30",
					)}
					style={
						{
							height: "var(--spin-h)",
							opacity: 0.22,
							animationDelay: `calc(var(--spin-dur) / ${SPINNER_BARS} * ${i})`,
						} as React.CSSProperties
					}
				/>
			))}
			<span className="sr-only">{label}…</span>
		</output>
	);
};
