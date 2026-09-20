import { tickCountAtom } from "#/collections/app";
import type { Measurement } from "#/collections/types";
import { sourceHeadlineMetric } from "#/components/terminal/kernel-meta";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { usePaintStore } from "#/components/ui/paint";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";

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

		const metric = sourceHeadlineMetric(measurement.source).slice(
			"metrics.".length,
		);
		const value = measurement.metrics?.[metric]?.normalized;

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
	const rootRef = usePaintStore(tickCountAtom, (tick) => ({
		fields: {
			tick: String(tick),
		},
	}));

	return (
		<Panel size="lg" ref={rootRef}>
			<Panel.Header
				title="System health"
				meta={
					<Badge
						label="live"
						variant="info"
						size="xs"
						className="font-mono font-semibold uppercase tracking-wide"
					/>
				}
			/>
			<Flex.Row className="mt-3 gap-4.5">
				<Flex.Column className="font-mono text-(--f1)">
					<Typography.Mono
						data-tick
						data-f="tick"
						size="m"
						tone="f1"
						className="text-[19px] leading-none font-normal"
					>
						—
					</Typography.Mono>
					<Typography.Span variant="f4" className="mt-1 font-mono text-[9px]">
						tick
					</Typography.Span>
				</Flex.Column>
			</Flex.Row>
		</Panel>
	);
};
