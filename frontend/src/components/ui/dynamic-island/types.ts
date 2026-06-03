import type { ReactNode } from "react";
import type { IslandAreaKey } from "@/components/ui/grid";

export type AreaKey = IslandAreaKey;

export type ShapeKey =
	| "pill"
	| "button"
	| "toast"
	| "form"
	| "card"
	| "nav"
	| "page";

export type ShapeConfig = {
	label: string;
	shellClassName: string;
	regions: Partial<Record<AreaKey, ReactNode>>;
};

export type DynamicIslandProps = ShapeConfig & {
	/** Identifies the morph target so regions enter/exit when the config changes. */
	morphKey?: string;
	className?: string;
};

export type DynamicIslandPresetProps = {
	shapeKey: ShapeKey;
	className?: string;
};
