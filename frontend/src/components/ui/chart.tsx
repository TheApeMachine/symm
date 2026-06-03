"use client";

import type React from "react";
import { cn } from "@/lib/utils";

export type ChartConfigEntry = {
	label?: string;
	color?: string;
	icon?: React.ComponentType;
	theme?: Record<string, string>;
};

export type ChartConfig = Record<string, ChartConfigEntry>;

interface ChartContainerProps {
	config: ChartConfig;
	className?: string;
	id?: string;
	children: React.ReactNode;
}

/*
ChartContainer wraps chart content with CSS custom properties derived from
the ChartConfig, making colors available as `--color-<key>` variables.
*/
export function ChartContainer({
	config,
	className,
	id,
	children,
}: ChartContainerProps) {
	const style = Object.fromEntries(
		Object.entries(config).flatMap(([key, entry]) =>
			entry.color ? [[`--color-${key}`, entry.color]] : [],
		),
	) as React.CSSProperties;

	return (
		<div
			id={id}
			className={cn("relative", className)}
			style={style}
			data-chart={id}
		>
			{children}
		</div>
	);
}

interface ChartStyleProps {
	id: string;
	config: ChartConfig;
}

/*
ChartStyle injects a <style> block that exposes chart config colors as CSS
custom properties scoped to the chart container element.
*/
export function ChartStyle({ id, config }: ChartStyleProps) {
	const rules = Object.entries(config)
		.filter(([, entry]) => entry.color)
		.map(([key, entry]) => `--color-${key}: ${entry.color};`)
		.join(" ");

	if (!rules) return null;

	return <style>{`[data-chart="${id}"] { ${rules} }`}</style>;
}
