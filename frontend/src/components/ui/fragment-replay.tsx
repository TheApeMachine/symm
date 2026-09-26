import { Badge } from "./badge";
import { Flex } from "./flex";
import { Typography } from "./typography";

export interface ReplayObservation {
	capture: { session: string; endpoint: string };
	cursor: { sequence: number | string; record: number };
	symbol: string;
	event: {
		a: { sequence: number | string };
		b: { sequence: number | string };
		c: { sequence: number | string };
		d: { sequence: number | string };
		excursion: number;
	};
}
export interface ReplayContext {
	symbol: string;
	vocabulary: string;
	tokens: string[];
	history?: string[];
	epoch: string;
	sequence: string;
}
export interface FragmentReplayProps {
	observation?: ReplayObservation;
	context?: ReplayContext;
	completed?: string;
	className?: string;
}

/* FragmentReplay shows the actual saved-cut cursor and region history used to train the model. */
export const FragmentReplay = ({
	observation,
	context,
	completed,
	className,
}: FragmentReplayProps) => (
	<Flex.Column
		className={`min-h-0 gap-4 overflow-auto border border-(--line) bg-(--surface) p-4 ${className ?? ""}`}
	>
		<Flex.Row className="items-center gap-4">
			<Typography.Label>Causal metric replay</Typography.Label>
			<Badge
				label={observation?.symbol ?? "Awaiting a complete fragment"}
				variant="info"
			/>
			<Typography.Mono>{completed ?? "—"} replay observations</Typography.Mono>
		</Flex.Row>
		<Typography.Mono>
			Epoch {observation?.capture.session ?? "—"} · metric sequence{" "}
			{observation?.cursor.sequence ?? "—"}
		</Typography.Mono>
		{observation && (
			<>
				<div className="grid grid-cols-4 border-y border-(--line) py-3">
					{[
						["Anchor", observation.event.a.sequence],
						["Ignition", observation.event.b.sequence],
						["Extremum", observation.event.c.sequence],
						["Confirmation", observation.event.d.sequence],
					].map(([label, value]) => (
						<Flex.Column key={label} className="gap-2">
							<Typography.Label>{label}</Typography.Label>
							<Typography.Mono>{value}</Typography.Mono>
						</Flex.Column>
					))}
				</div>
				<Typography.Mono>
					Recorded excursion {(observation.event.excursion * 100).toFixed(4)}%
				</Typography.Mono>
			</>
		)}
		<Typography.Label>Active region tokens</Typography.Label>
		<Typography.Mono className="break-all">
			{context?.tokens.join(" · ") ?? "Awaiting settled region tokens"}
		</Typography.Mono>
		<Typography.Label>Observed causal region history</Typography.Label>
		<Flex.Column className="gap-1">
			{context?.history?.map((token, index) => (
				<Typography.Mono key={`${index}:${token}`} className="break-all">
					{index + 1}. {token}
				</Typography.Mono>
			))}
		</Flex.Column>
		<Typography.Mono className="break-all text-(--f4)">
			Vocabulary {context?.vocabulary ?? "—"}
		</Typography.Mono>
	</Flex.Column>
);
