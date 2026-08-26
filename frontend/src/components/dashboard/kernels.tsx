import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import {
	DEFAULT_KERNELS,
	focusStore,
	measurementStore,
} from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

type QueryEntry = {
	status: HTMLElement | null;
	readout: HTMLElement | null;
};

const queryCache: Record<string, QueryEntry> = {};

export const KernelList = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	measurementStore.subscribe((state) => {
		if (!root.current) return;

		for (const kernel of DEFAULT_KERNELS) {
			const ring = state[kernel]?.[focusSymbol];
			if (!ring) continue;

			const last = ring.getLast();
			if (!last) continue;

			let element = queryCache[kernel];
			if (!element) {
				const cell = root.current.querySelector<HTMLElement>(`[data-kernel="${kernel}"]`);
				if (!cell) continue;

				element = {
					status: cell.querySelector<HTMLElement>('[data-k="status"]'),
					readout: cell.querySelector<HTMLElement>('[data-k="readout"]'),
				};
				queryCache[kernel] = element;
			}

			if (element.status) {
				element.status.textContent = "ONLINE";
			}


			if (element.readout) {
				let snrVal: number | null = null;
				for (let j = 0; j < last.metricsLength(); j++) {
					const m = last.metrics(j, metricObj);
					if (m && m.name() === "snr") {
						snrVal = m.raw();
						break;
					}
				}
				element.readout.textContent = snrVal !== null ? `snr: ${snrVal.toFixed(2)}` : "active";
			}
		}
	});

	return (
		<List ref={root} className="min-h-0 flex-1 border-(--line) border-b">
			{DEFAULT_KERNELS.map((kernel) => (
				<List.Item
					key={kernel}
					data-kernel={kernel}
					className="block border-(--line) border-b px-3 py-2.5"
				>
					<Flex.Row className="items-center justify-between gap-2">
						<Typography.Span className="truncate font-semibold text-[12.5px] text-(--f1)">
							{kernel}
						</Typography.Span>
						<Typography.Span
							data-k="status"
							className="shrink-0 rounded-xs border border-(--line2) bg-(--line) px-1.25 py-0.5 font-mono text-[9px] uppercase tracking-[0.07em]"
						>
							STANDBY
						</Typography.Span>
					</Flex.Row>
					<Flex.Row className="mt-1.5 items-center gap-2">
						<Typography.Span
							data-k="readout"
							className="min-w-0 flex-1 truncate font-mono text-[10px] text-(--f2)"
						>
							waiting
						</Typography.Span>
						<Typography.Span
							data-k="age"
							className="w-11.5 shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
						/>
					</Flex.Row>
				</List.Item>
			))}
		</List>
	);
};


