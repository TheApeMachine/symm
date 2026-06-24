import { cva } from "class-variance-authority";
import { type HTMLMotionProps, motion } from "motion/react";
import { cn } from "#/lib/utils";
import { type AppearVariant, appearPresets, type Measurement } from "./types";

const flexVariants = cva("flex", {
	variants: {
		direction: {
			row: "flex row",
			column: "flex col",
		},
		reverse: {
			true: "reverse",
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
			xxs: "gap xxs",
			xs: "gap xs",
			sm: "gap sm",
			unit: "gap unit",
			lg: "gap lg",
			xl: "gap xl",
			xxl: "gap xxl",
		},
		padding: {
			xxs: "pad xxs",
			xs: "pad xs",
			sm: "pad sm",
			unit: "pad unit",
			lg: "pad lg",
			xl: "pad xl",
			xxl: "pad xxl",
		},
		margin: {
			xxs: "margin xxs",
			xs: "margin xs",
			sm: "margin sm",
			unit: "margin unit",
			lg: "margin lg",
			xl: "margin xl",
			xxl: "margin xxl",
		},
		grow: {
			grow: "grow",
			shrink: "shrink",
		},
		fullHeight: {
			fullHeight: "height",
		},
		fullWidth: {
			fullWidth: "width",
		},
	},
});

interface FlexProps extends HTMLMotionProps<"div"> {
	direction?: "row" | "column";
	reverse?: boolean;
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
	gap?: Measurement;
	padding?: Measurement;
	margin?: Measurement;
	grow?: "grow" | "shrink";
	fullHeight?: boolean;
	fullWidth?: boolean;
	appear?: AppearVariant;
	children?: React.ReactNode;
}

const Flex = ({
	className,
	direction,
	reverse,
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
	children,
	...props
}: FlexProps) => {
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
					reverse: reverse ? true : undefined,
					fullHeight: fullHeight ? "fullHeight" : undefined,
					fullWidth: fullWidth ? "fullWidth" : undefined,
				}),
				className,
			)}
			{...preset}
			{...props}
		>
			{children ?? null}
		</motion.div>
	);
};

Flex.Row = ({ children, ...props }: FlexProps) => {
	return (
		<Flex direction="row" {...props}>
			{children}
		</Flex>
	);
};

Flex.Column = ({ children, ...props }: FlexProps) => {
	return (
		<Flex direction="column" {...props}>
			{children}
		</Flex>
	);
};

Flex.Center = ({ children, ...props }: FlexProps) => {
	return (
		<Flex justify="center" align="center" fullHeight fullWidth {...props}>
			{children}
		</Flex>
	);
};
