import { cva, type VariantProps } from "class-variance-authority";
import {
	type HTMLMotionProps,
	type Transition,
	motion,
} from "motion/react";
import type React from "react";
import { cn } from "@/lib/utils";

export { AnimatePresence } from "motion/react";

const springPanel: Transition = {
	type: "spring",
	stiffness: 380,
	damping: 32,
	mass: 0.7,
};
const easeFast: Transition = { duration: 0.18, ease: [0.22, 1, 0.36, 1] };

type AppearVariant =
	| "panelBottomRight"
	| "panelCenter"
	| "fade"
	| "fadeUp"
	| "fadeDown"
	| "slideRight"
	| "slideLeft"
	| "scaleIn"
	| "wiggleIdle"
	| "press";

const appearPresets: Record<
	AppearVariant,
	Pick<HTMLMotionProps<"div">, "initial" | "animate" | "exit" | "transition" | "whileHover" | "whileTap">
> = {
	panelBottomRight: {
		initial: { opacity: 0, scale: 0.85, y: 16 },
		animate: { opacity: 1, scale: 1, y: 0 },
		exit: { opacity: 0, scale: 0.9, y: 12 },
		transition: springPanel,
	},
	panelCenter: {
		initial: { opacity: 0, scale: 0.97 },
		animate: { opacity: 1, scale: 1 },
		exit: { opacity: 0, scale: 0.97 },
		transition: springPanel,
	},
	fade: {
		initial: { opacity: 0 },
		animate: { opacity: 1 },
		exit: { opacity: 0 },
		transition: easeFast,
	},
	fadeUp: {
		initial: { opacity: 0, y: 8 },
		animate: { opacity: 1, y: 0 },
		exit: { opacity: 0, y: -8 },
		transition: easeFast,
	},
	fadeDown: {
		initial: { opacity: 0, y: -8 },
		animate: { opacity: 1, y: 0 },
		exit: { opacity: 0, y: 8 },
		transition: easeFast,
	},
	slideRight: {
		initial: { opacity: 0, x: -24 },
		animate: { opacity: 1, x: 0 },
		exit: { opacity: 0, x: -24 },
		transition: springPanel,
	},
	slideLeft: {
		initial: { opacity: 0, x: 24 },
		animate: { opacity: 1, x: 0 },
		exit: { opacity: 0, x: 24 },
		transition: springPanel,
	},
	scaleIn: {
		initial: { opacity: 0, scale: 0.9 },
		animate: { opacity: 1, scale: 1 },
		exit: { opacity: 0, scale: 0.9 },
		transition: easeFast,
	},
	wiggleIdle: {
		animate: { rotate: [0, -6, 6, -3, 3, 0] },
		transition: {
			duration: 1.6,
			repeat: Number.POSITIVE_INFINITY,
			repeatDelay: 4,
			ease: "easeInOut",
		},
		whileHover: { scale: 1.06 },
		whileTap: { scale: 0.92 },
	},
	press: {
		whileHover: { scale: 1.04 },
		whileTap: { scale: 0.94 },
		transition: { type: "spring", stiffness: 400, damping: 22 },
	},
};

export const flexVariants = cva("flex", {
	variants: {
		direction: {
			row: "flex-row",
			column: "flex-col",
			rowReverse: "flex-row-reverse",
			columnReverse: "flex-col-reverse",
		},
		justify: {
			start: "justify-start",
			center: "justify-center",
			end: "justify-end",
			between: "justify-between",
			around: "justify-around",
			evenly: "justify-evenly",
			stretch: "justify-stretch",
		},
		align: {
			start: "items-start",
			center: "items-center",
			end: "items-end",
			baseline: "items-baseline",
			stretch: "items-stretch",
		},
		wrap: {
			wrap: "flex-wrap",
			wrapReverse: "flex-wrap-reverse",
			nowrap: "flex-nowrap",
		},
		gap: {
			1: "gap-1",
			2: "gap-2",
			3: "gap-3",
			4: "gap-4",
			5: "gap-5",
			6: "gap-6",
			7: "gap-7",
			8: "gap-8",
			9: "gap-9",
			10: "gap-10",
			11: "gap-11",
			12: "gap-12",
		},
		padding: {
			1: "p-1",
			2: "p-2",
			3: "p-3",
			4: "p-4",
			5: "p-5",
			6: "p-6",
			7: "p-7",
			8: "p-8",
			9: "p-9",
			10: "p-10",
			11: "p-11",
			12: "p-12",
		},
		margin: {
			1: "m-1",
			2: "m-2",
			3: "m-3",
			4: "m-4",
			5: "m-5",
			6: "m-6",
			7: "m-7",
			8: "m-8",
			9: "m-9",
			10: "m-10",
			11: "m-11",
			12: "m-12",
		},
		grow: {
			grow: "grow",
			shrink: "shrink",
			growShrink: "grow shrink",
			growShrinkGrow: "grow shrink grow",
			growShrinkShrink: "grow shrink shrink",
		},
		fullHeight: {
			fullHeight: "h-full",
		},
		fullWidth: {
			fullWidth: "w-full",
		},
	},
});

type FlexVariantProps = VariantProps<typeof flexVariants>;

export const Flex = ({
	children,
	className,
	direction,
	justify,
	align,
	wrap,
	gap,
	padding,
	margin,
	grow,
	fullHeight,
	fullWidth,
	appear,
	...props
}: HTMLMotionProps<"div"> &
	Omit<FlexVariantProps, "fullHeight" | "fullWidth"> & {
		direction?: "row" | "column" | "rowReverse" | "columnReverse";
		justify?:
			| "start"
			| "center"
			| "end"
			| "between"
			| "around"
			| "evenly"
			| "stretch";
		align?: "start" | "center" | "end" | "baseline" | "stretch";
		wrap?: "wrap" | "wrapReverse" | "nowrap";
		gap?: 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12;
		padding?: 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12;
		margin?: 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12;
		grow?:
			| "grow"
			| "shrink"
			| "growShrink"
			| "growShrinkGrow"
			| "growShrinkShrink";
		fullHeight?: boolean;
		fullWidth?: boolean;
		appear?: AppearVariant;
	}) => {
	const preset = appear ? appearPresets[appear] : undefined;

	return (
		<motion.div
			className={cn(
				flexVariants({
					direction,
					justify,
					align,
					wrap,
					gap,
					padding,
					margin,
					grow,
					fullHeight: fullHeight ? "fullHeight" : undefined,
					fullWidth: fullWidth ? "fullWidth" : undefined,
				}),
				className,
			)}
			{...preset}
			{...props}
		>
			{children}
		</motion.div>
	);
};

type FlexWithoutDirection = Omit<
	React.ComponentProps<typeof Flex>,
	"direction"
>;

Flex.Row = ({
	children,
	className,
	fullHeight,
	fullWidth,
	...props
}: FlexWithoutDirection) => {
	return (
		<Flex
			direction="row"
			fullHeight={fullHeight}
			fullWidth={fullWidth}
			{...props}
		>
			{children}
		</Flex>
	);
};

Flex.Column = ({
	children,
	className,
	fullHeight,
	fullWidth,
	...props
}: FlexWithoutDirection) => {
	return (
		<Flex
			direction="column"
			fullHeight={fullHeight}
			fullWidth={fullWidth}
			{...props}
		>
			{children}
		</Flex>
	);
};

Flex.Center = ({
	children,
	className,
	padding,
	...props
}: FlexWithoutDirection) => {
	return (
		<Flex
			justify="center"
			align="center"
			className={cn("h-full w-full", className)}
			{...props}
		>
			{children}
		</Flex>
	);
};
