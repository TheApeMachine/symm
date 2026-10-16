import { useEffect, useRef } from "react";
import { tickCountAtom } from "#/collections/app";
import { sourceHeadlineMetric } from "#/components/terminal/kernel-meta";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import type { Value } from "#/types/value";

export type TerminalHealth = {
	firing: number;
	measured: number;
	total: number;
	avg: number;
	label: string;
	tickMs: number;
	completed: boolean;
};

const healthLabel = (
	completed: boolean,
	firing: number,
	measured: number,
): string => {
	if (!completed || firing === 0) {
		return "Silent";
	}

	if (measured === 0) {
		return "Live · thin focus";
	}

	return "Live";
};

export const terminalHealthSummary = (
	measurements: Value[],
	focusSymbol: string,
	sources: string[],
	tick: { count: number; completed: boolean; ns: number },
): TerminalHealth => {
	const firingSources = new Set<string>();
	const focusSources = new Set<string>();
	const strengths: number[] = [];

	for (const measurement of measurements) {
		firingSources.add(measurement.origin);

		if (measurement.scope !== focusSymbol) {
			continue;
		}

		focusSources.add(measurement.origin);

		const metricName = sourceHeadlineMetric(measurement.origin).slice(
			"metrics.".length,
		);
		const found = measurement.metrics?.find((m) => m.name === metricName);
		const value = found ? (found.snm ?? found.raw) : undefined;

		if (typeof value === "number" && Number.isFinite(value)) {
			strengths.push(value);
		}
	}

	const avg =
		strengths.length === 0
			? 0
			: Math.round(
					(strengths.reduce((total, value) => total + value, 0) /
						strengths.length) *
						100,
				);

	return {
		firing: firingSources.size,
		measured: focusSources.size,
		total: sources.length,
		avg,
		label: healthLabel(tick.completed, firingSources.size, focusSources.size),
		tickMs: Math.round(tick.ns / 1_000_000),
		completed: tick.completed,
	};
};

export const HealthPanel = () => {
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const unsub = tickCountAtom.subscribe((tick) => {
			if (!root.current) return;
			const el = root.current.querySelector<HTMLElement>("[data-tick]");
			if (el) {
				el.textContent = String(tick);
			}
		});
		if (root.current) {
			const el = root.current.querySelector<HTMLElement>("[data-tick]");
			if (el) {
				el.textContent = String(tickCountAtom.get());
			}
		}
		return () => {
			unsub?.unsubscribe?.();
		};
	}, []);

	return (
		<Panel size="lg" ref={root}>
			<Flex.Row align="center" justify="between">
				<Flex className="font-semibold text-(--f1) text-xs">System health</Flex>
				<Badge
					label="live"
					className="rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
				/>
			</Flex.Row>
			<Flex.Row className="mt-3 gap-4.5">
				<div className="font-mono text-(--f1)">
					<span data-tick className="text-[19px] leading-none font-normal">
						—
					</span>
					<span className="mt-1 text-[9px] text-(--f4) font-mono">tick</span>
				</div>
			</Flex.Row>
		</Panel>
	);
};
