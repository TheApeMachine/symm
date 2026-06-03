"use client";

import { useReducedMotion } from "motion/react";
import {
	hidden as hiddenWithOffset,
	shown as shownVisible,
	spring as springTransition,
} from "#/components/ui/dynamic-island/animation";
import type { AreaKey } from "#/components/ui/dynamic-island/types";

const instant = { duration: 0 };

export const useIslandMotion = () => {
	const prefersReducedMotion = useReducedMotion();

	if (prefersReducedMotion) {
		return {
			layout: false,
			transition: instant,
			hidden: (_area: AreaKey) => shownVisible,
			shown: shownVisible,
			animatePresenceMode: "sync" as const,
		};
	}

	return {
		layout: true,
		transition: springTransition,
		hidden: hiddenWithOffset,
		shown: shownVisible,
		animatePresenceMode: "popLayout" as const,
	};
};
