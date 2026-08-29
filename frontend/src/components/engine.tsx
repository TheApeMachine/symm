import { useSelector } from "@tanstack/react-store";
import {
	measurementSourcesStore,
	positionStore,
	strategyStore,
	tickStore,
} from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";

const Row = ({ label, children }: { label: string; children: React.ReactNode }) => (
	<Flex.Row justify="between" align="center" className="gap-2">
		<Flex className="shrink-0 text-(--f4)">{label}</Flex>
		{children}
	</Flex.Row>
);

export const Engine = () => {
	const lastTick = useSelector(tickStore, (state) => state.getLast());
	const lastStrategy = useSelector(strategyStore, (state) =>
		state.findLast((f) => !!f.outcome() || f.decisionsLength() > 0),
	);
	const measurementCount = useSelector(
		measurementSourcesStore,
		(state) => state.length,
	);
	const lastPositions = useSelector(positionStore, (state) =>
		state.findLast((f) => f.rowsLength() > 0),
	);

	return (
		<Panel size="bare" className="p-2.5 font-mono text-[11px] leading-[1.7]">
			<Row label="seq">
				<Flex data-e="seq" className="text-(--f1)">
					{lastTick ? new Date(Number(lastTick.timestampNs() / 1000000n)).toISOString().slice(11, 19) : "—"}
				</Flex>
			</Row>
			<Row label="phase">
				<Flex data-e="phase" className="min-w-0 truncate text-(--acc)">
					{lastStrategy?.outcome() ?? "—"}
				</Flex>
			</Row>
			<Row label="cand">
				<Flex data-e="cand" className="text-(--f1)">
					{String(lastStrategy ? lastStrategy.decisionsLength() : "—")}
				</Flex>
			</Row>
			<Row label="meas">
				<Flex data-e="meas" className="text-(--f1)">
					{String(measurementCount)}
				</Flex>
			</Row>
			<Row label="open">
				<Flex data-e="open" className="text-(--f1)">
					{String(lastPositions ? lastPositions.rowsLength() : 0)}
				</Flex>
			</Row>
		</Panel>
	);
};


