import type { ClockSnapshot, HopSnapshot } from "#/collections/types";
import { Flex } from "#/components/ui/flex";

const MODULE_COPY: Record<string, string> = {
	price: "Price",
	desk: "Desk",
	crypto: "Crypto",
	collect: "Collect",
	commit: "Commit",
	cvd: "CVD",
	pumpdump: "Pump/Dump",
	depthflow: "Depthflow",
	exhaustion: "Exhaustion",
	hawkes: "Hawkes",
	toxicity: "Toxicity",
	correlation: "Correlation",
	leadlag: "Lead/Lag",
	liquidity: "Liquidity",
	sentiment: "Sentiment",
	category: "Category",
	resonance: "Resonance",
	manifold: "Manifold",
	causal: "Causal",
	cognition: "Cognition",
	graph: "Graph",
	planner: "Planner",
	mcts: "MCTS",
	allocation: "Allocation",
};

const SECTIONS: Array<{
	title: string;
	kind: string;
	hint: string;
}> = [
	{ title: "Trader / Broker", kind: "trader", hint: "ingress and execution" },
	{ title: "Signals", kind: "signal", hint: "one box per conditioner" },
	{ title: "Logic", kind: "logic", hint: "solver groups in order" },
	{ title: "Strategy", kind: "strategy", hint: "search, size, send" },
];

const EXTRA_KINDS: Record<string, string> = {
	broker: "trader",
	pipe: "trader",
};

export const formatNanos = (nanos: number): string => {
	if (!Number.isFinite(nanos) || nanos <= 0) {
		return "—";
	}

	if (nanos < 1_000) {
		return `${nanos.toFixed(0)}ns`;
	}

	if (nanos < 1_000_000) {
		return `${(nanos / 1_000).toFixed(1)}µs`;
	}

	if (nanos < 1_000_000_000) {
		return `${(nanos / 1_000_000).toFixed(2)}ms`;
	}

	return `${(nanos / 1_000_000_000).toFixed(2)}s`;
};

export const averageNanos = (clock: {
	count?: number;
	total_ns?: number;
}): number => {
	const count = clock.count ?? 0;

	if (count <= 0) {
		return 0;
	}

	return (clock.total_ns ?? 0) / count;
};

const hopBetween = (
	hops: HopSnapshot[],
	from: string,
	to: string,
): HopSnapshot =>
	hops.find((hop) => hop.from === from && hop.to === to) ?? {
		from,
		to,
		count: 0,
		total_ns: 0,
		last_ns: 0,
	};

const inboundHop = (
	hops: HopSnapshot[],
	name: string,
): HopSnapshot | undefined => hops.find((hop) => hop.to === name);

const ModuleCard = ({
	stage,
	inbound,
	atNs,
}: {
	stage: ClockSnapshot;
	inbound?: HopSnapshot;
	atNs: number;
}) => {
	const average = averageNanos(stage);
	const seen = (stage.last_at_ns ?? 0) > 0;
	const ageNs =
		seen && atNs > (stage.last_at_ns ?? 0) ? atNs - (stage.last_at_ns ?? 0) : 0;
	const hottest = seen && ageNs > 1_000_000_000;

	return (
		<div
			className={`min-w-0 rounded-md border px-2.5 py-2 ${
				hottest
					? "border-(--warn)/50 bg-(--surface)"
					: "border-(--line2) bg-(--surface)"
			}`}
		>
			<Flex.Row justify="between" align="baseline" className="gap-2">
				<span className="truncate font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f1)">
					{MODULE_COPY[stage.name] ?? stage.name}
				</span>
				<span className="shrink-0 font-mono text-[9px] tabular-nums text-(--f4)">
					{stage.count ?? 0}
				</span>
			</Flex.Row>
			<div className="mt-1 font-mono text-[13px] font-semibold tabular-nums text-(--acc)">
				{formatNanos(average)}
			</div>
			<Flex.Row
				justify="between"
				className="mt-0.5 font-mono text-[8.5px] text-(--f4)"
			>
				<span>work {formatNanos(stage.last_ns ?? 0)}</span>
				<span className={hottest ? "text-(--down)" : "text-(--f3)"}>
					age {formatAge(ageNs, seen)}
				</span>
			</Flex.Row>
			{inbound && (inbound.count ?? 0) > 0 ? (
				<div className="mt-0.5 font-mono text-[8.5px] text-(--warn)">
					in {formatNanos(averageNanos(inbound))}
					{(inbound.max_ns ?? 0) > 0
						? ` max ${formatNanos(inbound.max_ns ?? 0)}`
						: ""}
				</div>
			) : null}
		</div>
	);
};

const GroupHop = ({ hop }: { hop: HopSnapshot }) => (
	<div className="flex items-center justify-center py-1.5">
		<div className="h-px w-6 bg-(--line2)" />
		<span className="px-2 font-mono text-[9px] tabular-nums text-(--warn)">
			{formatNanos(averageNanos(hop))}
		</span>
		<div className="h-px w-6 bg-(--line2)" />
	</div>
);

const ModuleGrid = ({
	stages,
	hops,
	atNs,
}: {
	stages: ClockSnapshot[];
	hops: HopSnapshot[];
	atNs: number;
}) => (
	<div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
		{stages.map((stage) => (
			<ModuleCard
				key={stage.name}
				stage={stage}
				inbound={inboundHop(hops, stage.name)}
				atNs={atNs}
			/>
		))}
	</div>
);

/*
DiagnosticsSchema is the live module map. Each card is one named owner.
The amber "in" time is the wait from the previous owner onto this one.
*/
export const formatAge = (ageNs: number, seen: boolean): string => {
	if (!seen) {
		return "never";
	}

	if (ageNs <= 0) {
		return "now";
	}

	return formatNanos(ageNs);
};

export const DiagnosticsSchema = ({
	stages,
	hops,
	atNs = 0,
}: {
	stages: ClockSnapshot[];
	hops: HopSnapshot[];
	atNs?: number;
}) => {
	const grouped = new Map<string, ClockSnapshot[]>();

	for (const stage of stages) {
		const kind = EXTRA_KINDS[stage.kind ?? ""] ?? stage.kind ?? "pipe";
		const bucket = grouped.get(kind) ?? [];
		bucket.push(stage);
		grouped.set(kind, bucket);
	}

	return (
		<div className="flex flex-col gap-4">
			{SECTIONS.map((section, index) => {
				const cards = grouped.get(section.kind) ?? [];

				if (cards.length === 0) {
					return null;
				}

				const previous = index > 0 ? SECTIONS[index - 1] : null;
				const bridge =
					previous === null
						? null
						: hopBetween(
								hops,
								section.kind === "signal"
									? "crypto"
									: section.kind === "logic"
										? "commit"
										: "graph",
								section.kind === "signal"
									? "cvd"
									: section.kind === "logic"
										? "category"
										: "planner",
							);

				return (
					<div key={section.kind}>
						{bridge !== null ? <GroupHop hop={bridge} /> : null}
						<Flex.Row justify="between" align="baseline" className="mb-2">
							<span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f3)">
								{section.title}
							</span>
							<span className="font-mono text-[9px] uppercase tracking-wider text-(--f4)">
								{section.hint}
							</span>
						</Flex.Row>
						<ModuleGrid stages={cards} hops={hops} atNs={atNs} />
					</div>
				);
			})}
		</div>
	);
};
