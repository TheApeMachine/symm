import { useSelector } from "@tanstack/react-store";
import {
	measurementSourcesStore,
	positionStore,
	strategyStore,
	tickStore,
} from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

const Reading = ({
	label,
	which,
	value,
	accent = false,
}: {
	label?: string;
	which: string;
	value: string;
	accent?: boolean;
}) => (
	<Flex.Row align="center" gap={1}>
		{label ? <span>{label}</span> : null}
		<span
			data-read={which}
			className={cn(
				accent ? "text-(--acc)" : "font-semibold text-(--f1)",
				"tabular-nums",
			)}
		>
			{value}
		</span>
	</Flex.Row>
);

export const Pulse = () => {
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
		<Flex.Row
			align="center"
			gap={4}
			className="h-8 shrink-0 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)"
		>
			<Reading
				which="tick"
				value={lastTick ? new Date(Number(lastTick.timestampNs() / 1000000n)).toISOString().slice(11, 19) : "—"}
			/>
			<Reading
				label="phase"
				which="phase"
				accent
				value={lastStrategy?.outcome() ?? "—"}
			/>
			<Reading
				label="cand"
				which="cand"
				value={lastStrategy ? String(lastStrategy.decisionsLength()) : "—"}
			/>
			<Reading
				label="meas"
				which="meas"
				value={String(measurementCount)}
			/>
			<Reading
				label="open"
				which="open"
				value={String(lastPositions ? lastPositions.rowsLength() : 0)}
			/>
		</Flex.Row>
	);
};


