import { useSelector } from "@tanstack/react-store";
import { atoms } from "#/collections/app";
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
	const tick = useSelector(atoms["tickCount"], (state) => state);
	const phase = useSelector(atoms["phase"], (state) => state);
	const candidates = useSelector(atoms["candidates"], (state) => state);
	const signalCount = useSelector(
		atoms["signalSources"],
		(state) => state.length,
	);
	const openCount = useSelector(atoms["positionCount"], (state) => state);

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
			<Reading label="sigs" which="sigs" value={String(signalCount)} />
			<Reading label="open" which="open" value={String(openCount)} />
		</Flex.Row>
	);
};

