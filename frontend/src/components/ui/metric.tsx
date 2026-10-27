import { useSelector } from "@tanstack/react-store";
import { memo, useMemo, useRef } from "react";
import {
	atoms,
	kernelMeasurement,
	kernelSymbolForAggregate,
	signals,
} from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { sourceHeadline } from "#/components/terminal/kernel-meta";
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
import type { Value } from "#/types/value";

type StickyKernelCache = {
	byKernel: Record<string, Value>;
	bySymbol: Record<string, Record<string, Value>>;
};

const stickyKernelReading = (
	cache: StickyKernelCache,
	kernel: string,
	symbol: string,
	aggregate: Value | undefined,
): Value | null => {
	const next = kernelMeasurement(kernel, symbol, aggregate);
	if (next) {
		cache.byKernel[kernel] = next;
		const bySymbol = cache.bySymbol[symbol] ?? {};
		bySymbol[kernel] = next;
		cache.bySymbol[symbol] = bySymbol;
		return next;
	}

	return (
		cache.byKernel[kernel] ??
		cache.bySymbol[symbol]?.[kernel] ??
		null
	);
};

/** Latest folded kernel reading; holds the last sample when ticks omit this kernel. */
export const useKernelReading = (kernel: string): Value | null => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const aggregate = useSelector(
		signals.workspace,
		(state) => state.aggregate,
	);
	const stickyRef = useRef<StickyKernelCache>({
		byKernel: {},
		bySymbol: {},
	});

	const querySymbol = useMemo(
		() => kernelSymbolForAggregate(focusSymbol, aggregate),
		[focusSymbol, aggregate],
	);

	return useMemo(
		() =>
			stickyKernelReading(stickyRef.current, kernel, querySymbol, aggregate),
		[kernel, querySymbol, aggregate],
	);
};

export const stickyKernelMeasurements = (
	cache: StickyKernelCache,
	kernels: readonly string[],
	symbol: string,
	aggregate: Value | undefined,
): Record<string, Value | null> => {
	const out: Record<string, Value | null> = {};
	for (const kernel of kernels) {
		out[kernel] = stickyKernelReading(cache, kernel, symbol, aggregate);
	}
	return out;
};

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
					className="mt-0 min-h-0 flex-1"
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
	const measurement = useKernelReading(name);
	return <MetricRow name={name} measurement={measurement} compact={compact} />;
};
