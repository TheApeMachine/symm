import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo, useState } from "react";
import { focusAtom } from "#/collections/app";
import { type PaperRecord, paperStore } from "#/collections/paper";
import {
	type GrammarEvent,
	type LoggedChord,
	type LoggedEvent,
	type SequiturFeed,
	type SequiturFrame,
	sequiturStore,
} from "#/collections/sequitur";
import {
	Badge,
	Chip,
	Flex,
	Grid,
	Section,
	Toolbar,
	Typography,
} from "#/components/ui";
import { frameToSnapshot } from "./sequitur-engine";
import { SequiturLaminarStrata } from "./sequitur-laminar-strata";
import { SequiturRulebookDeck } from "./sequitur-rulebook-deck";
import { SequiturTelescopingTape } from "./sequitur-telescoping-tape";

/*
SequiturGrammarViz is the live view of strategy.Sequitur for the focused
symbol (strategy/sequitur.go): the grid gate that fires region tokens, the
chords appended, every structural step Sequitur took (create, reuse, inline),
the rule hierarchy, and the path the Trainer's policy is keyed on. Every
number is from the hub feed; nothing is simulated.
*/

// R0 grows without bound in a live session; the tape and strata show its tail.
export const ROOT_WINDOW = 48;
const EVENTS_SHOWN = 160;
const STALE_AFTER_MS = 10_000;

export const ruleName = (id: number) => (id === 0 ? "S" : `M${id}`);

export const describeEvent = (event: GrammarEvent): string => {
	switch (event.kind) {
		case "append":
			return `+ ${event.token ?? ""} appended to S`;
		case "create":
			return `${ruleName(event.rule)} ← ${event.digram.join(" ")} (repeat in ${ruleName(event.from)} and ${ruleName(event.into)})`;
		case "reuse":
			return `${event.digram.join(" ")} is ${ruleName(event.rule)}: substituted in ${ruleName(event.into)}`;
		case "inline":
			return `${ruleName(event.rule)} used once: inlined into ${ruleName(event.into)}`;
	}
};

const EVENT_VARIANT = {
	append: "disabled",
	create: "success",
	reuse: "info",
	inline: "warning",
} as const;

const useNow = (periodMs: number) => {
	const [now, setNow] = useState(() => Date.now());
	useEffect(() => {
		const handle = window.setInterval(() => setNow(Date.now()), periodMs);
		return () => window.clearInterval(handle);
	}, [periodMs]);
	return now;
};

const FeedStatus = ({ feed, now }: { feed?: SequiturFeed; now: number }) => {
	if (!feed) {
		return <Badge size="xs" variant="disabled" label="awaiting frames" />;
	}
	const age = now - feed.receivedAt;
	return age > STALE_AFTER_MS ? (
		<Badge
			size="xs"
			variant="warning"
			label={`no new chord for ${Math.round(age / 1000)}s`}
		/>
	) : (
		<Badge size="xs" variant="success" label="live" />
	);
};

/*
The chord-level surprise gate (strategy/gate.go): the resonance solver's
surprise on this tick, its causal z-score, and whether the chord the
midpoint rule proposed was appended.
*/
const SurpriseGate = ({ frame }: { frame: SequiturFrame }) => {
	const gate = frame.gate;

	if (!gate) {
		return (
			<div
				className="flex items-center gap-1.5 text-[10px] text-(--f4)"
				data-testid="surprise-gate"
			>
				<Badge size="xs" variant="disabled" label="gate off" />
				every proposed chord is appended
			</div>
		);
	}

	const verdict = !gate.warm
		? `cold: ${gate.cold}`
		: gate.passed
			? "passed"
			: "blocked";

	return (
		<div
			className="flex items-center gap-1.5 text-[10px] text-(--f3)"
			data-testid="surprise-gate"
		>
			<Badge
				size="xs"
				variant={gate.passed ? "success" : gate.warm ? "warning" : "disabled"}
				label={verdict}
			/>
			<span>surprise {gate.surprise.toPrecision(3)}</span>
			<span>· z {gate.z === null ? "undefined" : gate.z.toFixed(2)}</span>
		</div>
	);
};

/* The grid gate: active regions by brightness, the midpoint, what fired. */
const GatePanel = ({ frame }: { frame: SequiturFrame }) => {
	const peak = frame.active[0]?.brightness ?? 0;
	const fired = new Set(frame.fired);

	return (
		<Section fit="content" className="shrink-0">
			<Section.Header
				size="s"
				title="Grid gate"
				meta={`midpoint ${frame.midpoint.toPrecision(3)} · ${frame.fired.length}/${frame.active.length} fired · held ${frame.held} frame(s) before`}
			/>
			<div className="flex flex-col gap-0.5 p-2 bg-(--sunken)">
				<SurpriseGate frame={frame} />
				{frame.active.map((region) => {
					const token = `R${String(region.key).padStart(2, "0")}`;
					const width = peak > 0 ? (region.brightness / peak) * 100 : 0;
					const isFired = fired.has(token);
					return (
						<div
							key={token}
							className="grid grid-cols-[34px_1fr_64px] items-center gap-1.5 text-[10px]"
							data-testid={`gate-${token}`}
						>
							<span
								className={
									isFired ? "text-emerald-400 font-bold" : "text-(--f4)"
								}
							>
								{token}
							</span>
							<div className="relative h-2 bg-(--surface) rounded-[1px]">
								<div
									className={`absolute inset-y-0 left-0 ${isFired ? "bg-emerald-500/70" : "bg-(--line2)"}`}
									style={{ width: `${width}%` }}
								/>
								{peak > 0 && (
									<div
										className="absolute inset-y-[-2px] w-px bg-amber-400"
										style={{ left: `${(frame.midpoint / peak) * 100}%` }}
										title="midpoint of brightest and dimmest active region"
									/>
								)}
							</div>
							<span className="text-right text-(--f3)">
								{region.brightness.toPrecision(3)}
							</span>
						</div>
					);
				})}
			</div>
		</Section>
	);
};

/* The chords appended to S, oldest left, with the path each one left active. */
const ChordStream = ({ chords }: { chords: LoggedChord[] }) => (
	<Section fit="content" className="shrink-0">
		<Section.Header
			size="s"
			title="Chord stream"
			meta={`last ${chords.length} chords proposed · dashed: blocked by the surprise gate, not appended to S`}
		/>
		<div className="flex gap-1 overflow-x-auto p-2 bg-(--sunken)">
			{chords.map((chord) => (
				<div
					key={chord.seq}
					className={`flex flex-col items-center gap-0.5 px-1 py-0.5 border rounded-[2px] shrink-0 ${
						chord.blocked
							? "border-dashed border-(--line) opacity-50"
							: chord.stored
								? "border-emerald-500/60"
								: "border-(--line)"
					}`}
					data-testid={chord.blocked ? "chord-blocked" : "chord-appended"}
					title={`tick ${chord.tick}${chord.blocked ? " · blocked" : ` · path ${chord.path}`}${
						chord.gate
							? ` · surprise ${chord.gate.surprise.toPrecision(3)} z ${chord.gate.z === null ? "undefined" : chord.gate.z.toFixed(2)}`
							: ""
					}${chord.stored ? ` · policy ${chord.action || "disagrees"}` : ""}`}
				>
					{chord.fired.map((token) => (
						<span key={token} className="text-[9px] text-(--f2)">
							{token}
						</span>
					))}
					{chord.completed && (
						<span className="text-[8px] font-bold text-sky-400">
							{chord.completed}
						</span>
					)}
					<span className="text-[8px] text-(--f4)">#{chord.tick}</span>
				</div>
			))}
		</div>
	</Section>
);

/*
The active path, the deepest stored path the recent tokens end on (the
CognitionTree's prefix match, strategy/cognition.go), its action, and what
paper trading did with it.
*/
const MatchPanel = ({ frame }: { frame: SequiturFrame }) => {
	const { policy } = frame;
	const published = policy.epoch !== "0";
	return (
		<Section fit="content" className="shrink-0">
			<Section.Header
				size="s"
				title="Prefix match → paper desk"
				meta={
					published
						? `policy epoch ${policy.epoch} · ${policy.paths} paths`
						: "no policy published"
				}
			/>
			<div className="flex flex-col gap-1 p-2 bg-(--sunken) text-[10px]">
				<div className="flex items-center gap-1.5">
					<span className="text-(--f4)">completed</span>
					<span className="text-sky-400 font-bold">
						{frame.completed || "— (no rule closed)"}
					</span>
				</div>
				<div className="flex items-center gap-1.5">
					<span className="text-(--f4)">path</span>
					<span className="text-(--f1) break-all" data-testid="active-path">
						{frame.path}
					</span>
				</div>
				<div className="flex items-center gap-1.5">
					<span className="text-(--f4)">match</span>
					{!published ? (
						<Badge size="xs" variant="disabled" label="none loaded" />
					) : !policy.stored ? (
						<Badge size="xs" variant="disabled" label="no stored path" />
					) : (
						<>
							<span
								className="text-(--f1) break-all"
								data-testid="matched-path"
							>
								{policy.match}
							</span>
							<span className="text-(--f4)">depth {policy.depth}</span>
							{policy.action ? (
								<Badge
									size="xs"
									variant={
										policy.action === "enter"
											? "success"
											: policy.action === "exit"
												? "error"
												: "info"
									}
									label={policy.action}
								/>
							) : (
								<Badge
									size="xs"
									variant="warning"
									label="stored, actions disagree"
								/>
							)}
						</>
					)}
				</div>
				<div className="flex items-center gap-1.5">
					<span className="text-(--f4)">paper</span>
					{!policy.trading ? (
						<Badge size="xs" variant="disabled" label="inactive" />
					) : (
						<span className="text-(--f2)" data-testid="paper-outcome">
							{policy.outcome || "—"}
						</span>
					)}
				</div>
			</div>
		</Section>
	);
};

/* The paper round trips the run closed, newest first (data.PaperKeys). */
const PaperTrades = ({ records }: { records: PaperRecord[] }) => (
	<Section fit="content" className="shrink-0">
		<Section.Header
			size="s"
			title="Paper trades"
			meta={`${records.length} decision records`}
		/>
		<div className="max-h-40 overflow-y-auto divide-y divide-(--line) bg-(--sunken)">
			{records.length === 0 ? (
				<div className="px-2 py-1 text-[10px] text-(--f4)">
					no round trip closed yet
				</div>
			) : (
				records
					.slice()
					.reverse()
					.map((record) => (
						<div
							key={record.seq}
							className="grid grid-cols-[64px_38px_1fr_auto] items-center gap-1.5 px-2 py-0.5 text-[10px]"
							title={`fill ${record.fields.fill_price} vs intended ${record.fields.intended_price} · latency ${record.fields.latency_seconds.toFixed(3)}s · fees ${record.fields.fees}`}
						>
							<span className="text-(--f2) truncate">{record.symbol}</span>
							<Badge
								size="xxs"
								variant={record.action === "enter" ? "success" : "error"}
								label={record.action}
							/>
							<span className="text-(--f3) truncate">
								{record.path} · d{record.fields.depth}
							</span>
							<span
								className={
									record.fields.pnl >= 0 ? "text-emerald-400" : "text-rose-400"
								}
							>
								{(record.fields.net_return * 100).toFixed(3)}%
							</span>
						</div>
					))
			)}
		</div>
	</Section>
);

const EventLog = ({ events }: { events: LoggedEvent[] }) => {
	const shown = events.slice(-EVENTS_SHOWN).reverse();
	return (
		<Section fit="pane" className="flex-1 min-h-0 flex flex-col">
			<Section.Header
				size="s"
				title="Sequitur steps"
				meta={`newest first · ${events.length} kept`}
			/>
			<div className="flex-1 overflow-y-auto divide-y divide-(--line) bg-(--sunken)">
				{shown.map((event) => (
					<div
						key={event.seq}
						className="flex items-center gap-1.5 px-2 py-0.5 text-[10px]"
					>
						<Badge
							size="xxs"
							variant={EVENT_VARIANT[event.kind]}
							label={event.kind}
						/>
						<span className="text-(--f2) truncate" title={describeEvent(event)}>
							{describeEvent(event)}
						</span>
						<span className="ml-auto text-[8px] text-(--f4) shrink-0">
							#{event.tick}
						</span>
					</div>
				))}
			</div>
		</Section>
	);
};

export const SequiturGrammarViz = () => {
	const focus = useSelector(focusAtom, (s) => s);
	const feed = useSelector(sequiturStore, (state) => state[focus]);
	const paper = useSelector(paperStore, (state) => state);
	const now = useNow(1000);

	// Synchronized hover states across lenses
	const [hoveredRuleId, setHoveredRuleId] = useState<string | null>(null);
	const [hoveredCharIndex, setHoveredCharIndex] = useState<number | null>(null);
	const [hoveredDigramKey, setHoveredDigramKey] = useState<string | null>(null);

	const frame = feed?.latest;
	const snapshot = useMemo(
		() => (frame ? frameToSnapshot(frame, ROOT_WINDOW) : null),
		[frame],
	);
	const compression =
		frame && frame.rawLength > 0
			? frame.grammarSymbols / frame.rawLength
			: null;

	return (
		<Flex.Column
			fullWidth
			className="h-full min-h-0 bg-(--bg) text-(--f2) font-mono"
		>
			<Toolbar
				size="s"
				className="border-b border-(--line) bg-(--surface) px-3 gap-3"
			>
				<Flex.Row align="center" gap={2}>
					<Typography.Label
						size="s"
						tone="f1"
						className="font-bold tracking-wider"
					>
						Sequitur Grammar
					</Typography.Label>
					<Badge size="xs" variant="warning" label={focus} />
					<FeedStatus feed={feed} now={now} />
				</Flex.Row>

				<Toolbar.Spacer />

				<Flex.Row align="center" gap={2}>
					<Chip
						size="xs"
						label="Tick"
						value={frame ? String(frame.tick) : "—"}
					/>
					<Chip size="xs" label="Chords" value={String(feed?.frames ?? 0)} />
					<Chip
						size="xs"
						label="Rules"
						value={String(frame?.rules.length ?? 0)}
					/>
					<Chip
						size="xs"
						label="Symbols"
						value={
							frame
								? `${frame.grammarSymbols} / ${frame.rawLength} tokens`
								: "—"
						}
					/>
					<Chip
						size="xs"
						label="Compression"
						value={
							compression === null ? "—" : `${(compression * 100).toFixed(0)}%`
						}
					/>
					<Chip
						size="xs"
						label="Invariants"
						value={
							frame
								? frame.violations.length === 0
									? "hold"
									: `${frame.violations.length} violated`
								: "—"
						}
					/>
				</Flex.Row>
			</Toolbar>

			{!frame || !snapshot ? (
				<Flex.Center className="flex-1 text-(--f4) text-xs text-center px-6">
					awaiting sequitur frames for {focus}: they stream from `make run` once
					the grid lights regions for this symbol
				</Flex.Center>
			) : (
				<Grid
					responsive={false}
					fullWidth
					className="flex-1 min-h-0 min-w-0 grid-cols-[minmax(0,1fr)_360px]"
				>
					<Flex.Column className="min-h-0 border-r border-(--line) bg-(--sunken) overflow-y-auto">
						<GatePanel frame={frame} />
						<ChordStream chords={feed.chords} />
						{frame.violations.length > 0 && (
							<Section fit="content" className="shrink-0">
								<Section.Header
									size="s"
									title="Invariant violations"
									meta="checked by the backend"
								/>
								<div className="p-2 text-[10px] text-rose-400 bg-(--sunken)">
									{frame.violations.map((violation) => (
										<div key={violation}>{violation}</div>
									))}
								</div>
							</Section>
						)}
						<SequiturTelescopingTape
							snapshot={snapshot}
							hoveredRuleId={hoveredRuleId}
							onHoverRule={setHoveredRuleId}
							hoveredCharIndex={hoveredCharIndex}
							onHoverChar={setHoveredCharIndex}
							hoveredDigramKey={hoveredDigramKey}
							onHoverDigram={setHoveredDigramKey}
						/>
						<SequiturLaminarStrata
							snapshot={snapshot}
							hoveredRuleId={hoveredRuleId}
							onHoverRule={setHoveredRuleId}
							hoveredCharIndex={hoveredCharIndex}
							onHoverChar={setHoveredCharIndex}
						/>
					</Flex.Column>

					<Flex.Column className="min-h-0 bg-(--surface) overflow-hidden">
						<MatchPanel frame={frame} />
						<PaperTrades records={paper} />
						<EventLog events={feed.events} />
						<div className="h-[45%] min-h-0 border-t border-(--line)">
							<SequiturRulebookDeck
								snapshot={snapshot}
								hoveredRuleId={hoveredRuleId}
								onHoverRule={setHoveredRuleId}
								hoveredDigramKey={hoveredDigramKey}
								onHoverDigram={setHoveredDigramKey}
							/>
						</div>
					</Flex.Column>
				</Grid>
			)}
		</Flex.Column>
	);
};
