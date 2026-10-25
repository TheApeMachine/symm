import { useSelector } from "@tanstack/react-store";
import { memo, useMemo } from "react";
import { atoms, kernelMeasurement, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { sourceHeadline } from "#/components/terminal/kernel-meta";
import type { Value } from "#/types/value";
import {
	Badge,
	Button,
	Component,
	Flex,
	Meter,
	Sparkline,
	Typography,
} from "#/components/ui";
import { cn } from "#/lib/utils";

interface MetricProps {
	name: string;
	measurement: Value | null;
	compact?: boolean;
}

export const MetricRow = memo(
	({ name, measurement, compact = false }: MetricProps) => {
	const headline = sourceHeadline(name) ?? name;

	return (
		<Component
			value={measurement}
			className="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<Button
				variant="bare"
				shape="block"
				className="min-h-0 justify-start border-(--line) border-b"
				onClick={() => {
					atoms.focusMetric.set(() => name);
					terminalStore.actions.inspectSource(name);
				}}
				fullHeight
			>
				<Flex.Row
					align="center"
					justify="between"
					gap={2}
					padding={1}
					fullWidth
					className="shrink-0"
				>
					<Typography.Span
						variant="f1"
						semibold
						truncate
						className={cn(compact && "text-[10px]")}
					>
						{name.toUpperCase()}
					</Typography.Span>
					<Badge
						data-k="badge"
						data-metric-badge={headline}
						label="Standby"
						variant="disabled"
						size="xxs"
					/>
				</Flex.Row>
				<Sparkline
					data-k="sparkline"
					data-series={headline}
					title={`${name} sparkline`}
					className="mt-0 h-0 min-h-0"
				/>
				<Flex.Row
					align="center"
					gap={2}
					padding={1}
					fullWidth
					className="shrink-0"
				>
					<Meter
						data-metric-meter={headline}
						data-k="bar"
						layout="bar"
						size="xxs"
						percent={0}
						variant="warning"
						className="flex-1"
						title={`${name} headline: ${headline}`}
					/>
					<Typography.Mono
						data-metric-raw={headline}
						data-format="fixed:3"
						data-k="value"
						size="xxs"
						tone="f2"
						className="w-11 shrink-0 text-right font-mono"
					>
						--
					</Typography.Mono>
				</Flex.Row>
			</Button>
		</Component>
	);
});

MetricRow.displayName = "MetricRow";

/** Standalone metric with its own workspace subscription (tests, compact layouts). */
export const Metric = ({
	name,
	compact = false,
}: {
	name: string;
	compact?: boolean;
}) => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const tick = useSelector(
		signals.workspace,
		(state) => state.aggregate?.tick ?? 0n,
	);
	const measurement = useMemo(
		() =>
			kernelMeasurement(name, focusSymbol, signals.workspace.state.aggregate),
		[name, focusSymbol, tick],
	);
	return <MetricRow name={name} measurement={measurement} compact={compact} />;
};
