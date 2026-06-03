"use client";

import { DynamicIsland } from "#/components/ui/dynamic-island/component";
import { SHAPES } from "#/components/ui/dynamic-island/shapes";
import type { DynamicIslandPresetProps } from "#/components/ui/dynamic-island/types";

export const DynamicIslandPreset = ({
	shapeKey,
	className,
}: DynamicIslandPresetProps) => {
	const preset = SHAPES[shapeKey];

	return (
		<DynamicIsland morphKey={shapeKey} className={className} {...preset} />
	);
};
