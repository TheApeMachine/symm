"use client";

import { AnimatePresence } from "motion/react";
import type { ReactNode } from "react";
import type {
	AreaKey,
	DynamicIslandProps,
} from "#/components/ui/dynamic-island/types";
import { useIslandMotion } from "#/components/ui/dynamic-island/use-island-motion";
import { Grid } from "@/components/ui/grid";
import { cn } from "@/lib/utils";

export const DynamicIsland = ({
	morphKey,
	className,
	shellClassName,
	regions,
}: DynamicIslandProps) => {
	const islandMotion = useIslandMotion();
	const entries = Object.entries(regions) as [AreaKey, ReactNode][];

	return (
		<Grid.Island
			layout={islandMotion.layout}
			transition={islandMotion.transition}
			className={cn(
				"relative overflow-hidden border bg-card text-card-foreground shadow-lg",
				shellClassName,
				className,
			)}
			data-slot="dynamic-island"
		>
			<AnimatePresence mode={islandMotion.animatePresenceMode}>
				{entries.map(([area, node]) => (
					<Grid.IslandArea
						key={morphKey ? `${morphKey}-${area}` : area}
						area={area}
						layout={islandMotion.layout}
						transition={islandMotion.transition}
						initial={islandMotion.hidden(area)}
						animate={islandMotion.shown}
						exit={islandMotion.hidden(area)}
					>
						{node}
					</Grid.IslandArea>
				))}
			</AnimatePresence>
		</Grid.Island>
	);
};
