import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { measurementsStore, useSubscribe, measurementIdentity } from "#/providers/ws-stores";
import { Flex } from "@/components/ui/flex";

export const KernelList = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(
		measurementsStore,
		(state) => {
			for (const kernel of kernels) {
				const row =
					state.measurements[
						measurementIdentity({ source: kernel, symbol: focusSymbol })
					]?.latest();

				if (row === undefined || row === null || typeof row !== "object") {
					continue;
				}

				const cell = root.current?.querySelector<HTMLElement>(
					`[data-kernel="${kernel}"]`,
				);

				if (cell === null || cell === undefined) {
					continue;
				}

				const set = (q: string, value: string) => {
					const el = cell.querySelector<HTMLElement>(`[data-k="${q}"]`);

					if (el instanceof HTMLElement) {
						el.textContent = value;
					}
				};

				const record = row as Record<string, unknown>;
				set("source", String(record.source ?? ""));
				set("status", String(record.status ?? "STANDBY"));
				set("readout", String(record.readout ?? "waiting"));
				set("age", String(record.age ?? ""));
			}
		},
		[kernels, focusSymbol],
	);

	return (
		<List ref={root} className="min-h-0 flex-1 border-(--line) border-b">
			{kernels.map((kernel) => (
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
