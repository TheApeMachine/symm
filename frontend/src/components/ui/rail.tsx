import { cva, type VariantProps } from "class-variance-authority";
import { type HTMLMotionProps, motion } from "motion/react";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { cn } from "@/lib/utils";

export const railVariants = cva("flex min-h-0 flex-col", {
	variants: {
		position: {
			left: "border-(--line) border-r",
			right: "border-(--line) border-l",
			none: "",
		},
		surface: {
			surface: "bg-(--surface)",
			sunken: "bg-(--sunken)",
			bg: "bg-(--bg)",
			transparent: "bg-transparent",
		},
		width: {
			narrow: "w-[230px] shrink-0",
			default: "w-[280px] shrink-0",
			wide: "w-[320px] shrink-0",
			xwide: "w-[364px] shrink-0",
			auto: "w-auto",
			full: "w-full",
		},
	},
	defaultVariants: {
		position: "none",
		surface: "surface",
		width: "auto",
	},
});

type RailVariantProps = VariantProps<typeof railVariants>;

export type RailProps = Omit<HTMLMotionProps<"div">, "children"> &
	RailVariantProps & {
		children?: ReactNode;
	};

/**
 * Rail is a vertical container for side rails, inspectors, and tool strips across surfaces.
 */
export const Rail = ({
	position,
	surface,
	width,
	className,
	children,
	...props
}: RailProps) => {
	return (
		<motion.div
			className={cn(railVariants({ position, surface, width }), className)}
			{...props}
		>
			{children}
		</motion.div>
	);
};

export type RailHeaderProps = ComponentPropsWithoutRef<"div"> & {
	title?: ReactNode;
	meta?: ReactNode;
	children?: ReactNode;
};

Rail.Header = ({
	title,
	meta,
	className,
	children,
	...props
}: RailHeaderProps) => {
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-2.5",
				className,
			)}
			{...props}
		>
			{title !== undefined ? (
				<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em] select-none">
					{title}
				</span>
			) : null}
			{children}
			{meta !== undefined ? (
				<span className="font-mono text-[9.5px] text-(--f4)">{meta}</span>
			) : null}
		</div>
	);
};

export type RailBodyProps = ComponentPropsWithoutRef<"div"> & {
	padding?: "none" | "s" | "m" | "lg";
	children?: ReactNode;
};

const railBodyPadding: Record<NonNullable<RailBodyProps["padding"]>, string> = {
	none: "",
	s: "p-2",
	m: "p-3.5 space-y-3.5",
	lg: "p-4 space-y-4",
};

Rail.Body = ({
	padding = "none",
	className,
	children,
	...props
}: RailBodyProps) => {
	return (
		<div
			className={cn(
				"min-h-0 flex-1 overflow-auto",
				railBodyPadding[padding],
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};

Rail.Footer = ({
	className,
	children,
	...props
}: ComponentPropsWithoutRef<"div">) => {
	return (
		<div
			className={cn(
				"mt-auto shrink-0 border-(--line) border-t p-3.5",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};
