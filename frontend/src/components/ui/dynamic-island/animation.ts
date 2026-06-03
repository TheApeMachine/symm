import type { AreaKey } from "#/components/ui/dynamic-island/types";

export const spring = {
	type: "spring" as const,
	stiffness: 420,
	damping: 40,
	mass: 1,
};

const OFFSET: Record<AreaKey, { x?: number; y?: number; scale?: number }> = {
	h: { y: -28 },
	f: { y: 28 },
	n: { x: -28 },
	a: { x: 28 },
	m: { scale: 0.9 },
};

export const hidden = (area: AreaKey) => ({ opacity: 0, ...OFFSET[area] });

export const shown = { opacity: 1, x: 0, y: 0, scale: 1 };
