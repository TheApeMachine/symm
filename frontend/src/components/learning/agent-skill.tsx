import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { focusAtom, type RingBuffer, signals } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { memoizedQuery } from "#/lib/utils";
import type { WireMeasurement } from "#/types/capnp/measurement";
import { basis, percent } from "./format";

export const AgentSkill = () => {
	const symbol = useSelector(focusAtom, (s) => s);
	const ref = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const root = ref.current;
		if (!root) return;

		const update = (ring: RingBuffer<WireMeasurement>) => {
			if (!ring || ring.isEmpty()) return;
			const len = ring.getBufferLength();

			for (let i = 0; i < len; i++) {
				const m = ring.get(i);
				if (!m) continue;
				const metricMap: Record<string, number> = {};
				for (const metric of m.metrics ?? []) {
					if (metric?.name) metricMap[String(metric.name)] = metric.raw ?? 0;
				}
				const edge = metricMap.edge ?? 0;
				const winRate = metricMap.win_rate ?? 0;
				const resolved = metricMap.resolved ?? 0;

				const winRateEl = memoizedQuery(
					root,
					'[data-a="winrate"]',
				) as HTMLElement;
				if (winRateEl) {
					winRateEl.innerText = resolved > 0 ? percent(winRate) : "—";
				}

				const edgeEl = memoizedQuery(root, '[data-a="edge"]') as HTMLElement;
				if (edgeEl) {
					edgeEl.innerText = resolved > 0 ? basis(edge) : "—";
				}
			}
		};

		const getTrainingRing = (
			records?: Record<string, RingBuffer<WireMeasurement>>,
		): RingBuffer<WireMeasurement> | null => {
			if (!records) {
				return null;
			}

			return (
				records[symbol] ??
				records.learner ??
				records[""] ??
				Object.values(records)[0] ??
				null
			);
		};

		const initial = getTrainingRing(signals.training?.state);
		if (initial) {
			update(initial);
		}

		const unsub = signals.training.subscribe((state: any) => {
			const activeRing = getTrainingRing(state);
			if (activeRing) {
				update(activeRing);
			}
		});

		return () => {
			unsub?.unsubscribe?.();
		};
	}, [symbol]);

	return (
		<Flex.Row ref={ref} align="center" gap={6}>
			<Badge label="Model" variant="info" dot />
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Win Rate
				</Typography.Label>
				<Typography.Mono size="lg" tone="f1" data-a="winrate">
					—
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Edge
				</Typography.Label>
				<Typography.Mono size="lg" tone="accent" data-a="edge">
					—
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
