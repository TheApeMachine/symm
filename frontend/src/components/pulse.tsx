import { useSelector } from "@tanstack/react-store";
import {
	candidatesAtom,
	measurementSourcesAtom,
	phaseAtom,
	positionCountAtom,
	tickCountAtom,
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
	const tick = useSelector(tickCountAtom, (state) => state);
	const phase = useSelector(phaseAtom, (state) => state);
	const candidates = useSelector(candidatesAtom, (state) => state);
	const measurementCount = useSelector(
		measurementSourcesAtom,
		(state) => state.length,
	);
	const openCount = useSelector(positionCountAtom, (state) => state);

	return (
		<Flex.Row
			align="center"
			gap={4}
			className="h-8 shrink-0 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)"
		>
			<Reading which="tick" value={tick ? String(tick) : "—"} />
			<Reading label="phase" which="phase" accent value={phase} />
			<Reading
				label="cand"
				which="cand"
				value={candidates ? String(candidates) : "—"}
			/>
			<Reading label="meas" which="meas" value={String(measurementCount)} />
			<Reading label="open" which="open" value={String(openCount)} />
		</Flex.Row>
	);
};
