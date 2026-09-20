import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import {
	focusMetric,
	focusStore,
	type RingBuffer,
	SIGNALS,
	signals,
} from "#/collections/app";
import { RingCursor } from "#/collections/ring";
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

		const cursor = new RingCursor<MeasurementT>();
		const values: number[] = [];
		let observedRing: RingBuffer<MeasurementT> | undefined;
		let generation = -1;

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (observedRing !== ring || generation !== ring.generation) {
				values.length = 0;
				observedRing = ring;
				generation = ring.generation;
			}
			if (!ring || ring.isEmpty()) {
				setBadge(badgeEl, "disabled", "STANDBY");
				setMeter(barEl, 0, "disabled");
				setSparkline(sparkEl, areaEl, [], false);
				return;
			}

			cursor.read(ring, (measurement) => {
				const snr = measurement.snr;
				values.push(snr);
				if (values.length > ring.getSize()) values.shift();

				renderValue(valueEl, snr);
				setBadge(badgeEl, "success", "HEALTHY");

				if (barEl) {
					const conf = Math.min(1, Math.max(0, measurement.maturity || snr));
					setMeter(barEl, conf * 100, "brand");
				}
			});
			btn.dataset.dropped = String(cursor.dropped);

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
			className="border-(--line) border-b"
			data-kernel={source}
			onClick={() => {
				focusMetric.setState(() => source);
				terminalStore.actions.inspectSource(source);
			}}
		>
			<Flex.Row align="center" justify="between" gap={2} padding={2} fullWidth>
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
			<Flex.Row align="center" gap={2} padding={2} fullWidth>
				<Meter
					data-k="bar"
					layout="bar"
					size="xxs"
					percent={0}
					variant="warning"
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
	sources = SIGNALS,
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
