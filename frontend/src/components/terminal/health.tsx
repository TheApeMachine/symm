import type { Measurement } from "#/collections/types";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";
import { Badge } from "@/components/ui/badge";
import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";

/*
TerminalHealth is the state of the measurement pipeline for one tick.
*/
export type TerminalHealth = {
	firing: number;
	measured: number;
	total: number;
	avg: number;
	label: string;
	tickMs: number;
	completed: boolean;
};

/*
terminalHealthSummary reduces a measurement batch to the health of the pipeline
that produced it.

Firing counts the sources reporting anywhere in the universe, while measured
counts those reporting for the focused symbol. Separating them keeps a quiet
focus symbol from reading as a dead pipeline when the rest of the book is live.
*/
export const terminalHealthSummary = (
	measurements: Measurement[],
	focusSymbol: string,
	sources: string[],
	tick: { count: number; completed: boolean; ns: number },
): TerminalHealth => {
	const firingSources = new Set<string>();
	const focusSources = new Set<string>();
	const strengths: number[] = [];

	for (const measurement of measurements) {
		firingSources.add(measurement.source);

		if (measurement.symbol !== focusSymbol) {
			continue;
		}

		focusSources.add(measurement.source);

		const value = measurement.normalized ?? measurement.raw;

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

/*
healthLabel names the pipeline state. A universe that is reporting while the
focused symbol is not is thin rather than silent, because the engine is working
and only this one view is quiet.
*/
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

/*
HealthPanel is the system-health shell.

The tick counter is painted from the engine's own tick frame, so the panel
shows the pipeline advancing even on ticks that produced no decision.
*/
export const HealthPanel = () => (
	<Component registerKey="tick">
		{({ ref, className }) => (
			<Panel size="lg" ref={ref} className={className}>
				<Flex.Row align="center" justify="between">
					<Flex className="font-semibold text-(--f1) text-xs">
						System health
					</Flex>
					<Badge
						label="live"
						className="rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
					/>
				</Flex.Row>
				<Flex.Row className="mt-3 gap-4.5">
					<Stat
						value={<Typography.Span data-paint="count" />}
						label="tick"
						emphasis="default"
						valueClassName="font-normal"
					/>
				</Flex.Row>
			</Panel>
		)}
	</Component>
);
