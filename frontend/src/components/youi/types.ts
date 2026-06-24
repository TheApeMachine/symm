import type { HTMLMotionProps, Transition } from "motion/react";

export type Measurement = "xxs" | "xs" | "sm" | "unit" | "lg" | "xl" | "xxl";

const springPanel: Transition = {
	type: "spring",
	stiffness: 380,
	damping: 32,
	mass: 0.7,
};

const easeFast: Transition = { duration: 0.18, ease: [0.22, 1, 0.36, 1] };

export type AppearVariant =
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

export const appearPresets: Record<
	AppearVariant,
	Pick<
		HTMLMotionProps<"div">,
		"initial" | "animate" | "exit" | "transition" | "whileHover" | "whileTap"
	>
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

