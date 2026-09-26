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

export interface PulseProps {
	observations?: string | number;
	phase?: string;
	decisions?: number | string;
	open?: number | string;
}

export const Pulse = ({ observations, phase, decisions, open }: PulseProps) => {
	return (
		<Flex.Row
			align="center"
			gap={4}
			className="h-8 shrink-0 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)"
		>
			<Reading which="tick" value={String(observations ?? "—")} />
			<Reading label="phase" which="phase" accent value={phase ?? "—"} />
			<Reading
				label="decisions"
				which="cand"
				value={String(decisions ?? "—")}
			/>
			<Reading label="open" which="open" value={String(open ?? "—")} />
		</Flex.Row>
	);
};
