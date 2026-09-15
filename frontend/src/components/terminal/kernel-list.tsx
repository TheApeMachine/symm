import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import {
	DEFAULT_KERNELS,
	focusMetric,
	focusStore,
	type RingBuffer,
	signals,
} from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	Badge,
	Button,
	Flex,
	Meter,
	Sparkline,
	setBadge,
	setMeter,
	setSparkline,
	Typography,
} from "#/components/ui";
import { cn, memoizedQuery, renderValue } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

const KernelRow = ({
	source,
	compact,
}: {
	source: string;
	compact: boolean;
}) => {
	const symbol = useSelector(focusStore, (s) => s);
	const rowRef = useRef<HTMLButtonElement>(null);

	useEffect(() => {
		const btn = rowRef.current;
		if (!btn) return;

		const badgeEl = memoizedQuery(btn, '[data-k="badge"]') as HTMLElement;
		const areaEl = memoizedQuery(btn, '[data-k="area"]') as SVGPolylineElement;
		const sparkEl = memoizedQuery(
			btn,
			'[data-k="spark"]',
		) as SVGPolylineElement;
		const barEl = memoizedQuery(btn, '[data-k="bar"]') as HTMLElement;
		const valueEl = memoizedQuery(btn, '[data-k="value"]') as HTMLElement;

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) {
				setBadge(badgeEl, "disabled", "STANDBY");
				setMeter(barEl, 0, "disabled");
				setSparkline(sparkEl, areaEl, [], false);
				return;
			}

			const values: number[] = [];
			const len = ring.getBufferLength();

			for (let i = 0; i < len; i++) {
				const measurement = ring.get(i);
				if (measurement === undefined) {
					continue;
				}

				const snr = measurement.snr;
				values.push(snr);

				renderValue(valueEl, snr);
				setBadge(badgeEl, "success", "HEALTHY");

				if (barEl) {
					const conf = Math.min(1, Math.max(0, measurement.maturity || snr));
					setMeter(barEl, conf * 100, "brand");
				}
			}

			setSparkline(sparkEl, areaEl, values, true);
		};

		const initial =
			signals[source]?.state[symbol] ?? signals[source]?.state[""];

		if (initial) {
			update(initial);
		}

		const unsubSignal = signals[source]?.subscribe((state) => {
			const r = state[symbol] ?? state[""];

			if (r !== undefined) {
				update(r);
			}
		});

		return () => {
			unsubSignal?.unsubscribe?.();
		};
	}, [source, symbol]);

	return (
		<Button
			ref={rowRef}
			variant="bare"
			shape="block"
			data-kernel={source}
			onClick={() => {
				focusMetric.setState(() => source);
				terminalStore.actions.inspectSource(source);
			}}
		>
			<Flex.Row align="center" justify="between" gap={2} fullWidth>
				<Typography.Span
					variant="f1"
					semibold
					truncate
					className={cn(compact && "text-[10px]")}
				>
					{source.toUpperCase()}
				</Typography.Span>
				<Badge data-k="badge" label="Standby" variant="disabled" size="xxs" />
			</Flex.Row>
			<Sparkline data-k="sparkline" title={`${source} sparkline`} />
			<Flex.Row align="center" gap={2} fullWidth>
				<Meter
					data-k="bar"
					layout="bar"
					size="xxs"
					percent={0}
					variant="info"
					className="flex-1"
					title={`${source} confidence`}
				/>
				<Typography.Mono
					data-k="value"
					size="xxs"
					tone="f2"
					className="w-11 shrink-0 text-right font-mono"
				>
					--
				</Typography.Mono>
			</Flex.Row>
		</Button>
	);
};

export type KernelListProps = {
	sources?: string[];
	compact?: boolean;
};

export const KernelList = ({
	sources = DEFAULT_KERNELS,
	compact = false,
}: KernelListProps = {}) => {
	return (
		<Flex.Column
			className={cn("min-h-0 flex-1 overflow-auto", compact && "text-[10px]")}
		>
			{sources.map((source) => (
				<KernelRow key={source} source={source} compact={compact} />
			))}
		</Flex.Column>
	);
};
