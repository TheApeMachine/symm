import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

/*
Slider is a range control styled from theme tokens.

Like Input, the primitive itself strips the chrome down to the track and thumb.
When placed inside Slider.Field, it takes on a box with leading label/icon and
trailing value readout, matching the rhythm of the rest of the controls.
*/

export const sliderVariants = cva(
	[
		"w-full cursor-pointer appearance-none bg-transparent outline-none",
		"disabled:cursor-not-allowed disabled:opacity-50",
		"[&::-webkit-slider-runnable-track]:rounded-full [&::-webkit-slider-runnable-track]:bg-(--line)",
		"[&::-moz-range-track]:rounded-full [&::-moz-range-track]:bg-(--line)",
		"[&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full",
		"[&::-webkit-slider-thumb]:border [&::-webkit-slider-thumb]:border-(--line2)",
		"[&::-webkit-slider-thumb]:transition-transform hover:[&::-webkit-slider-thumb]:scale-110",
		"[&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border [&::-moz-range-thumb]:border-(--line2)",
	],
	{
		variants: {
			size: {
				xs: [
					"h-3",
					"[&::-webkit-slider-runnable-track]:h-1",
					"[&::-moz-range-track]:h-1",
					"[&::-webkit-slider-thumb]:size-2.5 [&::-webkit-slider-thumb]:-mt-[3px]",
					"[&::-moz-range-thumb]:size-2.5",
				],
				s: [
					"h-4",
					"[&::-webkit-slider-runnable-track]:h-1.5",
					"[&::-moz-range-track]:h-1.5",
					"[&::-webkit-slider-thumb]:size-3.5 [&::-webkit-slider-thumb]:-mt-[4px]",
					"[&::-moz-range-thumb]:size-3.5",
				],
				m: [
					"h-5",
					"[&::-webkit-slider-runnable-track]:h-2",
					"[&::-moz-range-track]:h-2",
					"[&::-webkit-slider-thumb]:size-4.5 [&::-webkit-slider-thumb]:-mt-[5px]",
					"[&::-moz-range-thumb]:size-4.5",
				],
			},
			tone: {
				acc: [
					"accent-(--acc)",
					"[&::-webkit-slider-thumb]:bg-(--acc)",
					"[&::-moz-range-thumb]:bg-(--acc)",
				],
				brand: [
					"accent-(--brand)",
					"[&::-webkit-slider-thumb]:bg-(--brand)",
					"[&::-moz-range-thumb]:bg-(--brand)",
				],
				info: [
					"accent-(--info)",
					"[&::-webkit-slider-thumb]:bg-(--info)",
					"[&::-moz-range-thumb]:bg-(--info)",
				],
				success: [
					"accent-(--success)",
					"[&::-webkit-slider-thumb]:bg-(--success)",
					"[&::-moz-range-thumb]:bg-(--success)",
				],
				warn: [
					"accent-(--warn)",
					"[&::-webkit-slider-thumb]:bg-(--warn)",
					"[&::-moz-range-thumb]:bg-(--warn)",
				],
				error: [
					"accent-(--error)",
					"[&::-webkit-slider-thumb]:bg-(--error)",
					"[&::-moz-range-thumb]:bg-(--error)",
				],
			},
		},
		defaultVariants: {
			size: "s",
			tone: "acc",
		},
	},
);

export type SliderProps = Omit<ComponentProps<"input">, "size"> &
	VariantProps<typeof sliderVariants>;

export const Slider = ({
	ref,
	size,
	tone,
	className,
	min = 0,
	max = 100,
	step = 1,
	...props
}: SliderProps) => (
	<input
		ref={ref}
		type="range"
		min={min}
		max={max}
		step={step}
		className={cn(sliderVariants({ size, tone }), className)}
		{...props}
	/>
);

export const sliderFieldVariants = cva("flex items-center", {
	variants: {
		variant: {
			box: "rounded-[3px] border border-(--line) bg-(--surface) px-2.5 py-1",
			bare: "",
		},
		gap: {
			s: "gap-2",
			m: "gap-3",
		},
	},
	defaultVariants: {
		variant: "box",
		gap: "s",
	},
});

export type SliderFieldProps = ComponentProps<"div"> &
	VariantProps<typeof sliderFieldVariants> & {
		leading?: ReactNode;
		trailing?: ReactNode;
	};

Slider.Field = ({
	ref,
	variant,
	gap,
	leading,
	trailing,
	className,
	children,
	...props
}: SliderFieldProps) => (
	<div
		ref={ref}
		className={cn(sliderFieldVariants({ variant, gap }), className)}
		{...props}
	>
		{leading}
		{children}
		{trailing}
	</div>
);
