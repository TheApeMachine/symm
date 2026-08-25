import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { measurementsStore, positionsStore, strategyStore, tickStore, useSubscribe } from "#/providers/ws-stores";

const Row = ({ label, children }: { label: string; children: React.ReactNode }) => (
	<Flex.Row justify="between" align="center" className="gap-2">
		<Flex className="shrink-0 text-(--f4)">{label}</Flex>
		{children}
	</Flex.Row>
);

export const Engine = () => {
	const tick = useSubscribe(tickStore, (state) => {
		const el = tick.current?.querySelector<HTMLElement>("[data-e=seq]");
		if (el instanceof HTMLElement) el.textContent = String(state?.count ?? "—");
	});
	const strategy = useSubscribe(strategyStore, (state) => {
		const phase = strategy.current?.querySelector<HTMLElement>("[data-e=phase]");
		const cand = strategy.current?.querySelector<HTMLElement>("[data-e=cand]");
		if (phase instanceof HTMLElement) phase.textContent = state?.outcome ?? "—";
		if (cand instanceof HTMLElement) cand.textContent = String(state === null ? "—" : state.decisions.length);
	});
	const measurements = useSubscribe(measurementsStore, (state) => {
		const el = measurements.current?.querySelector<HTMLElement>("[data-e=meas]");
		if (el instanceof HTMLElement) el.textContent = String(Object.keys(state.measurements).length);
	});
	const positions = useSubscribe(positionsStore, (state) => {
		const el = positions.current?.querySelector<HTMLElement>("[data-e=open]");
		if (el instanceof HTMLElement) el.textContent = String(Object.keys(state.positions).length);
	});

	return (
		<Panel size="bare" className="p-2.5 font-mono text-[11px] leading-[1.7]">
			<div ref={tick}>
				<Row label="seq">
					<Flex data-e="seq" className="text-(--f1)">—</Flex>
				</Row>
			</div>
			<div ref={strategy}>
				<Row label="phase">
					<Flex data-e="phase" className="min-w-0 truncate text-(--acc)">—</Flex>
				</Row>
				<Row label="cand">
					<Flex data-e="cand" className="text-(--f1)">—</Flex>
				</Row>
			</div>
			<div ref={measurements}>
				<Row label="meas">
					<Flex data-e="meas" className="text-(--f1)">—</Flex>
				</Row>
			</div>
			<div ref={positions}>
				<Row label="open">
					<Flex data-e="open" className="text-(--f1)">—</Flex>
				</Row>
			</div>
		</Panel>
	);
};
