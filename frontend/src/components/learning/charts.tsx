import { useEffect, useRef, useState } from "react";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import type { LearningMarkT } from "#/providers/telemetry/telemetry/learning-mark";
import type { LearningTrackT } from "#/providers/telemetry/telemetry/learning-track";
import { amount, basis, clock, duration, percent } from "./format";
import type {
	Influence,
	LearningEvent,
	LearningView,
	Skill,
	Token,
	Wallet,
} from "./state";

/*
Shared visual vocabulary for the learning surface.

These are pictures of measurements that already exist elsewhere on this page as
numbers: nothing here computes an estimate, substitutes a reading or invents a
value where one is absent. An absent measurement is drawn as absent — an empty
track, a hollow cell — never as a zero, because "no evidence yet" and "measured
as zero" mean opposite things to someone reading the shape rather than the
figure.

Colour is used for direction only — up/down, present/absent — and never to
judge a reading as good or bad.
*/

/* share keeps a proportion inside its track without inventing one from a zero total. */
const share = (part: number, whole: number) =>
	whole > 0 ? Math.max(0, Math.min(1, part / whole)) : 0;

/*
PipelineFunnel answers "where does the flow stop?" — the single question the
counts in the header cannot answer by sitting next to each other.

The stages span several orders of magnitude: observations arrive continuously,
decisions are taken from them, and only the small fraction whose measurement
window has closed becomes evidence. A bar drawn proportionally would leave
every stage after the first invisible, so the bar length is logarithmic and
the honest number — what fraction of the stage above survives — is written on
each row in full.
*/
export const PipelineFunnel = ({ view }: { view: LearningView | null }) => {
	const stages = [
		{
			key: "observed",
			label: "Market observations",
			count: view?.steps ?? 0,
			note: "Every update the agent saw",
		},
		{
			key: "decided",
			label: "Decisions taken",
			count: view?.decisions ?? 0,
			note: "Observations it chose to act on, waiting included",
		},
		{
			key: "dispatched",
			label: "Intents sent to the account",
			count: view?.dispatched ?? 0,
			note: "Simulated orders filled through the normal position regulator",
		},

		{
			key: "resolved",
			label: "Outcomes measured",
			count: view?.resolved ?? 0,
			note: "A trade leg must close and be persisted before it supplies a completed grade",
		},
	];

	const largest = Math.max(...stages.map((stage) => stage.count), 1);
	const width = (count: number) =>
		count > 0
			? Math.max(2, (Math.log10(1 + count) / Math.log10(1 + largest)) * 100)
			: 0;

	return (
		<Flex.Column className="gap-2 p-3">
			{stages.map((stage, index) => {
				const previous = index === 0 ? undefined : stages[index - 1];
				const carried =
					previous && previous.count > 0
						? stage.count / previous.count
						: undefined;
				const stalled = carried !== undefined && stage.count === 0;

				return (
					<Flex.Column key={stage.key} className="gap-1">
						<Flex.Row align="center" className="justify-between gap-3">
							<Typography.Mono size="s" tone={stalled ? "f3" : "f1"}>
								{stage.label}
							</Typography.Mono>
							<Typography.Mono size="s" tone="f2">
								{stage.count.toLocaleString()}
								{carried === undefined
									? ""
									: ` · ${percent(carried)} of the step above`}
							</Typography.Mono>
						</Flex.Row>
						<div className="h-3 w-full overflow-hidden rounded-[3px] bg-(--line)">
							<div
								className="h-full rounded-[3px] transition-[width] duration-500"
								style={{
									width: `${width(stage.count)}%`,
									background: stalled ? "var(--warn)" : "var(--info)",
								}}
							/>
						</div>
						<Typography.Mono size="s" tone="f4">
							{stalled
								? `Nothing reaches this step yet · ${stage.note}`
								: stage.note}
						</Typography.Mono>
					</Flex.Column>
				);
			})}
			<Typography.Mono size="s" tone="f4" className="pt-1">
				Bar length is logarithmic — each equal step of bar is ten times as many.
				The percentage beside each step is the honest one: how much of the step
				above it survives.
			</Typography.Mono>
		</Flex.Column>
	);
};

const RHYTHM_TONE: Record<string, string> = {
	issued: "var(--info)",
	filled: "var(--up)",
	waited: "var(--f4)",
	rejected: "var(--warn)",
	resolved: "var(--acc)",
	recycled: "var(--error)",
};

/*
EventRhythm draws the journal as a picture instead of a list. One row per kind
of thing that can happen, one tick per occurrence, time running left to right
over exactly the window the journal retains.

Read it for texture: a row that never fires is a step of the machine that is
not running, and a row that fires in bursts is a machine reacting to the tape
rather than to a clock. Neither reading requires knowing what any number means.
*/
export const EventRhythm = ({ events }: { events: LearningEvent[] }) => {
	const stamped = events
		.map((event) => ({ event, at: Date.parse(event.at) }))
		.filter((entry) => Number.isFinite(entry.at))
		.sort((left, right) => left.at - right.at);

	if (stamped.length === 0) {
		return (
			<Typography.Mono className="p-3 text-(--f3)">
				The journal is empty. Nothing has been recorded to draw yet.
			</Typography.Mono>
		);
	}

	const first = stamped[0].at;
	const last = stamped[stamped.length - 1].at;
	const span = last - first;
	const kinds = Array.from(new Set(stamped.map((entry) => entry.event.kind)));

	return (
		<Flex.Column className="gap-2 px-3">
			<Typography.Mono size="s" tone="f4">
				{stamped.length.toLocaleString()} recorded moments over{" "}
				{span > 0 ? duration(span * 1e6) : "one instant"}
			</Typography.Mono>
			<Flex.Column className="gap-1">
				{kinds.map((kind) => {
					const ticks = stamped.filter((entry) => entry.event.kind === kind);
					return (
						<Flex.Row key={kind} align="center" gap={2}>
							<Typography.Mono
								size="s"
								tone="f3"
								className="w-20 shrink-0 truncate"
							>
								{kind}
							</Typography.Mono>
							<div className="relative h-5 flex-1 rounded-[3px] bg-(--sunken) border border-(--line)">
								{ticks.map((entry) => (
									<div
										key={`${entry.event.lane}-${entry.event.id}-${entry.event.kind}-${entry.at}`}
										className="absolute top-0.75 bottom-0.75 w-px"
										style={{
											left: `${span > 0 ? ((entry.at - first) / span) * 100 : 50}%`,
											background: RHYTHM_TONE[kind] ?? "var(--f3)",
										}}
										title={`${clock(entry.event.at)} · ${entry.event.mode} ${entry.event.lane + 1} · #${entry.event.id}`}
									/>
								))}
							</div>
							<Typography.Mono
								size="s"
								tone="f4"
								className="w-10 shrink-0 text-right"
							>
								{ticks.length}
							</Typography.Mono>
						</Flex.Row>
					);
				})}
			</Flex.Column>
			<Flex.Row className="justify-between font-mono text-[9px] text-(--f4)">
				<span>{clock(stamped[0].event.at)}</span>
				<span>{clock(stamped[stamped.length - 1].event.at)}</span>
			</Flex.Row>
		</Flex.Column>
	);
};

/* OutcomeRange locates the signed empirical mean relative to zero. */
export const OutcomeRange = ({ skill }: { skill?: Skill }) => {
	if (!skill?.defined)
		return <Typography.Mono>No completed outcomes yet.</Typography.Mono>;
	const extent = Math.abs(skill.mean);
	const width = extent > 0 ? 50 : 0;
	return (
		<Flex.Column className="gap-2 p-3">
			<Typography.Label>Completed decision benefit</Typography.Label>
			<div className="relative h-8 bg-(--sunken)">
				<div className="absolute left-1/2 h-full border-l border-(--line)" />
				<div
					className="absolute h-full"
					style={{
						left: skill.mean < 0 ? `${50 - width}%` : "50%",
						width: `${width}%`,
						background: skill.mean < 0 ? "var(--down)" : "var(--up)",
					}}
				/>
			</div>
			<Typography.Mono>
				{basis(-extent)} ← 0 → {basis(extent)} · mean {basis(skill.mean)}
			</Typography.Mono>
		</Flex.Column>
	);
};

/*
ImpulseBars redraws the impulse table's strength column as length. The table
underneath keeps every figure; this only makes the ordering and the size of
the gaps between quantities visible at a glance.
*/
export const ImpulseBars = ({ impulse }: { impulse: Token[] | null }) => {
	const tokens = (impulse ?? []).slice(0, 12);
	const strongest = Math.max(...tokens.map((token) => token.strength), 0);

	if (tokens.length === 0) return null;

	return (
		<Flex.Column className="gap-1 border-(--line) border-b p-3">
			{tokens.map((token) => (
				<Flex.Row key={token.token} align="center" gap={2}>
					<Typography.Mono
						size="s"
						tone="f2"
						className="w-44 shrink-0 truncate"
						title={`${token.source} / ${token.label}`}
					>
						{token.source} <span className="text-(--f4)">/ {token.label}</span>
					</Typography.Mono>
					<div className="relative h-3 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
						<div
							className="h-full bg-(--acc)"
							style={{ width: `${share(token.strength, strongest) * 100}%` }}
						/>
						{/* Authority is drawn inside the same bar: how much of this strength is evidenced. */}
						<div
							className="absolute top-0 bottom-0 w-px bg-(--f1)"
							style={{
								left: `${share(token.strength, strongest) * token.authority * 100}%`,
							}}
							title={`Evidenced authority ${percent(token.authority)}`}
						/>
					</div>
					<Typography.Mono
						size="s"
						tone="f4"
						className="w-24 shrink-0 text-right"
					>
						{amount(token.strength)}
					</Typography.Mono>
				</Flex.Row>
			))}
			<Typography.Mono size="s" tone="f4">
				Bar length is how hot the quantity is right now. The pale line inside it
				marks how much of that heat is backed by evidence rather than by a
				single fresh reading.
			</Typography.Mono>
		</Flex.Column>
	);
};

/*
InfluenceGrid is the discovery table as a matrix: one row per observed context prefix,
one column per action it has evidence for. Colour direction says whether the
outcome that followed was up or down, opacity says how strong, and the small
inner square says how much authority stands behind it.

A blank cell is a pairing that has never resolved. That emptiness is itself the
reading — it is where the agent has not looked yet.
*/
export const InfluenceGrid = ({
	influence,
}: {
	influence: Influence[] | null;
}) => {
	const measured = (influence ?? []).filter((entry) => entry.prior.Defined);
	if (measured.length === 0) return null;

	const actions = Array.from(
		new Set(measured.map((entry) => entry.action)),
	).sort();
	const rows = Array.from(
		measured
			.reduce(
				(accumulator, entry) => {
					const key = `${entry.token}`;
					const existing = accumulator.get(key);
					if (existing) {
						existing.entries.push(entry);
						existing.authority = Math.max(
							existing.authority,
							entry.prior.Authority,
						);
						return accumulator;
					}
					accumulator.set(key, {
						token: entry.token,
						source: entry.source,
						label: entry.label,
						authority: entry.prior.Authority,
						entries: [entry],
					});
					return accumulator;
				},
				new Map<
					string,
					{
						token: number;
						source: string;
						label: string;
						authority: number;
						entries: Influence[];
					}
				>(),
			)
			.values(),
	)
		.sort((left, right) => right.authority - left.authority)
		.slice(0, 14);

	const strongest = Math.max(
		...measured.map((entry) => Math.abs(entry.prior.Mean)),
		0,
	);

	return (
		<Flex.Column className="gap-2 border-(--line) border-b p-3">
			<Flex.Row align="center" gap={2}>
				<span className="w-44 shrink-0" />
				{actions.map((label) => (
					<Typography.Mono
						key={label}
						size="s"
						tone="f4"
						className="flex-1 truncate text-center"
					>
						{label}
					</Typography.Mono>
				))}
			</Flex.Row>
			{rows.map((row) => (
				<Flex.Row key={row.token} align="center" gap={2}>
					<Typography.Mono
						size="s"
						tone="f2"
						className="w-44 shrink-0 truncate"
						title={`${row.source} / ${row.label}`}
					>
						{row.source} <span className="text-(--f4)">/ {row.label}</span>
					</Typography.Mono>
					{actions.map((label) => {
						const cell = row.entries.find((entry) => entry.action === label);
						if (!cell) {
							return (
								<div
									key={label}
									className="h-6 flex-1 rounded-[3px] border border-(--line) border-dashed"
									title={`${row.source} / ${row.label} · ${label}: never resolved here`}
								/>
							);
						}
						const magnitude = share(Math.abs(cell.prior.Mean), strongest);
						return (
							<div
								key={label}
								className="relative h-6 flex-1 overflow-hidden rounded-[3px] border border-(--line)"
								title={`${row.source} / ${row.label} · ${label}\nMean outcome ${basis(cell.prior.Mean)}\nAuthority ${percent(cell.prior.Authority)}\n${cell.prior.Samples} samples`}
							>
								<div
									className="absolute inset-0"
									style={{
										background:
											cell.prior.Mean >= 0 ? "var(--up)" : "var(--down)",
										opacity: 0.12 + 0.8 * magnitude,
									}}
								/>
								<div
									className="absolute bottom-0 left-0 h-0.75 bg-(--f1) opacity-70"
									style={{ width: `${cell.prior.Authority * 100}%` }}
								/>
							</div>
						);
					})}
				</Flex.Row>
			))}
			<Typography.Mono size="s" tone="f4">
				Colour is direction: what followed this quantity when that action was
				taken. Stronger colour is a larger measured move; the pale strip along
				the bottom is how much evidence stands behind it. A dashed cell has
				never resolved — the agent has not tried that pairing here.
			</Typography.Mono>
		</Flex.Column>
	);
};

export const WalletBars = ({ lanes }: { lanes: Wallet[] | null }) => {
	const wallets = lanes ?? [];
	if (wallets.length === 0) return null;

	const extent = Math.max(
		...wallets.map((wallet) => Math.abs(wallet.profit)),
		...wallets.map((wallet) => Math.abs(wallet.realized)),
		1e-9,
	);

	return (
		<Flex.Column className="gap-1 border-(--line) border-b p-3">
			{wallets.map((wallet) => {
				const value = wallet.profit;
				const magnitude =
					value === undefined ? 0 : share(Math.abs(value), extent) * 50;
				const positive = (value ?? 0) >= 0;
				return (
					<Flex.Row key={wallet.lane} align="center" gap={2}>
						<Typography.Mono
							size="s"
							tone={wallet.mode === "policy" ? "accent" : "f3"}
							className="w-28 shrink-0 truncate"
						>
							{wallet.mode} {wallet.lane + 1}
						</Typography.Mono>
						<div className="relative h-4 flex-1 rounded-[3px] bg-(--sunken) border border-(--line)">
							<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--line2)" />
							{value === undefined ? (
								<span className="absolute inset-0 flex items-center justify-center font-mono text-[9px] text-(--f4)">
									not valued yet
								</span>
							) : (
								<div
									className="absolute top-1 bottom-1 rounded-xs"
									style={{
										width: `${magnitude}%`,
										left: positive ? "50%" : undefined,
										right: positive ? undefined : "50%",
										background: positive ? "var(--up)" : "var(--down)",
									}}
								/>
							)}
						</div>
						<Typography.Mono
							size="s"
							tone="f4"
							className="w-24 shrink-0 text-right"
						>
							{value === undefined ? "—" : amount(value)}
						</Typography.Mono>
					</Flex.Row>
				);
			})}
			<Typography.Mono size="s" tone="f4">
				This run's profit for each wallet, on one shared axis. Each wallet owns
				its own capital. Losses remain in that wallet.
			</Typography.Mono>
		</Flex.Column>
	);
};

export const MeasurementWindow = ({ view }: { view: LearningView | null }) => (
	<Flex.Column className="gap-2 p-3">
		<Typography.Label>Observation history</Typography.Label>
		<Typography.Mono>
			{duration(view?.horizonNs ?? 0)} of producer-supplied temporal context
		</Typography.Mono>
		<Typography.Mono>
			Decision outcomes wait for a completed, persisted trade leg. There is no
			spread-crossing or promotion gate.
		</Typography.Mono>
	</Flex.Column>
);

/* LearningProgress plots the actual completed and pending decision counts. */
export const LearningProgress = ({ view }: { view: LearningView | null }) => {
	const trained = view?.forward?.trained ?? 0;
	const pending =
		view?.agents.reduce((total, member) => total + Number(member.pending), 0) ??
		0;
	const history = useRef<Array<{ at: number; trained: number }>>([]);
	const [, redraw] = useState(0);

	useEffect(() => {
		if (!view) return;
		const samples = history.current;
		const latest = samples.at(-1);

		if (latest?.trained === trained) {
			return;
		}
		if (latest && trained < latest.trained) samples.length = 0;
		// This rate measures counts observed by this mounted dashboard. Producer
		// timestamps may be uninitialized or regress across market sources.
		samples.push({ at: performance.now(), trained });

		if (samples.length > 600) {
			samples.splice(0, samples.length - 600);
		}
		redraw((tick) => tick + 1);
	}, [trained, view]);

	const samples = history.current;
	const first = samples[0];
	const last = samples.at(-1);
	const span = first && last ? (last.at - first.at) / 1000 : 0;
	const rate =
		span > 0 && last ? ((last.trained - first.trained) / span) * 60 : null;
	const peak = Math.max(...samples.map((sample) => sample.trained), 1);

	const width = 520;
	const height = 150;
	const path = samples
		.map((sample, index) => {
			const x = (index / Math.max(samples.length - 1, 1)) * width;
			const y = height - (sample.trained / peak) * (height - 8);

			return `${index === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`;
		})
		.join(" ");

	return (
		<Flex.Column className="h-full w-full gap-2 px-3">
			<Flex.Row align="center" className="justify-between">
				<Flex.Column className="gap-0">
					<Typography.Mono size="lg" tone="accent">
						{trained.toLocaleString()} moves learned from
					</Typography.Mono>
					<Typography.Mono size="s" tone="f4">
						{rate === null
							? "measuring the rate"
							: `${rate.toFixed(1)} per minute over the last ${duration(span * 1e9)}`}
					</Typography.Mono>
				</Flex.Column>
				<Flex.Column className="items-end gap-0">
					<Typography.Mono size="s" tone="f3">
						{pending.toLocaleString()} awaiting a completed grade
					</Typography.Mono>
					<Typography.Mono size="s" tone="f4" className="max-w-64 truncate">
						{view?.status || "—"}
					</Typography.Mono>
				</Flex.Column>
			</Flex.Row>

			<div className="relative h-36 w-full overflow-hidden rounded bg-(--sunken) border border-(--line)">
				{samples.length < 2 ? (
					<span className="absolute inset-0 flex items-center justify-center font-mono text-[10px] text-(--f4)">
						Watching. The line appears as soon as the count changes.
					</span>
				) : (
					<svg
						viewBox={`0 0 ${width} ${height}`}
						className="h-full w-full select-none"
						preserveAspectRatio="none"
						role="img"
						aria-label="Confirmed moves the agent has learned from"
					>
						<title>Confirmed moves the agent has learned from</title>
						<path
							d={`${path} L ${width} ${height} L 0 ${height} Z`}
							fill="var(--acc)"
							opacity="0.15"
						/>
						<path d={path} fill="none" stroke="var(--acc)" strokeWidth="2" />
					</svg>
				)}
			</div>

			<Typography.Mono size="s" tone="f4">
				Completed decisions graded against persisted trade legs, using the
				original observations and action. Learning continues as tape outcomes
				arrive.
			</Typography.Mono>
		</Flex.Column>
	);
};

/*
DecisionRing shows what has become of the decisions this desk has made.

Every decision is in exactly one of three states, and which one dominates says
something different about the system. A ring full of settled decisions means the
tape is teaching; a ring full of waiting ones means decisions are being made
faster than they can be answered; an empty ring means nothing is happening at
all. Three counts side by side say the same thing, but the shape says it without
having to be read.
*/
export const DecisionRing = ({ view }: { view: LearningView | null }) => {
	const traders = view?.desk?.traders ?? [];
	const graded = traders.reduce((total, trader) => total + trader.graded, 0);
	const open = traders.reduce((total, trader) => total + trader.open, 0);
	const decisions = traders.reduce(
		(total, trader) => total + trader.decisions,
		0,
	);

	/*
		Whatever is neither settled nor waiting was made and then superseded.
		It is shown rather than folded into one of the other two, because a desk
		losing most of its decisions this way looks identical to a healthy one
		if the difference is hidden.
	*/
	const dropped = Math.max(0, decisions - graded - open);
	const total = graded + open + dropped;

	const slices = [
		{
			key: "graded",
			label: "graded by the tape",
			value: graded,
			tone: "var(--up)",
		},
		{
			key: "open",
			label: "awaiting a grade",
			value: open,
			tone: "var(--info)",
		},
		{ key: "dropped", label: "superseded", value: dropped, tone: "var(--f4)" },
	];

	const radius = 52;
	const circumference = 2 * Math.PI * radius;
	let offset = 0;

	return (
		<Flex.Row align="center" gap={4} className="flex-wrap p-3">
			<svg
				viewBox="0 0 140 140"
				className="h-32 w-32 shrink-0"
				role="img"
				aria-label="What became of the decisions this desk has made"
			>
				<title>What became of the decisions this desk has made</title>
				<circle
					cx="70"
					cy="70"
					r={radius}
					fill="none"
					stroke="var(--line)"
					strokeWidth="14"
				/>
				{total > 0 &&
					slices.map((slice) => {
						const length = (slice.value / total) * circumference;
						const dash = `${length} ${circumference - length}`;
						const rotation = (offset / circumference) * 360 - 90;
						offset += length;

						return (
							<circle
								key={slice.key}
								cx="70"
								cy="70"
								r={radius}
								fill="none"
								stroke={slice.tone}
								strokeWidth="14"
								strokeDasharray={dash}
								transform={`rotate(${rotation} 70 70)`}
							>
								<title>{`${slice.label}: ${slice.value}`}</title>
							</circle>
						);
					})}
				<text
					x="70"
					y="68"
					textAnchor="middle"
					fill="var(--f1)"
					fontSize="20"
					fontFamily="monospace"
				>
					{total > 0 ? percent(graded / total) : "—"}
				</text>
				<text
					x="70"
					y="84"
					textAnchor="middle"
					fill="var(--f4)"
					fontSize="9"
					fontFamily="monospace"
				>
					answered
				</text>
			</svg>
			<Flex.Column className="min-w-0 gap-1">
				{slices.map((slice) => (
					<Flex.Row key={slice.key} align="center" gap={2}>
						<span
							className="h-2.5 w-2.5 shrink-0 rounded-[2px]"
							style={{ background: slice.tone }}
						/>
						<Typography.Mono size="s" tone="f2">
							{slice.value.toLocaleString()} {slice.label}
						</Typography.Mono>
					</Flex.Row>
				))}
				<Typography.Mono size="s" tone="f4" className="max-w-72">
					{total === 0
						? "No decision has been made yet, so there is nothing to have become of."
						: `Of ${total.toLocaleString()} decisions across every wallet, this is how many the tape has already answered.`}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};

/*
TraderQuality puts every wallet on one axis, so the disagreement between them is
visible as a shape rather than as a column of signed numbers.

They are meant to disagree — that is why there is more than one. What matters is
whether any of them is consistently finding something the others are not, and
that is a comparison, not a reading.
*/
export const TraderQuality = ({ view }: { view: LearningView | null }) => {
	const traders = view?.desk?.traders ?? [];

	if (traders.length === 0) {
		return null;
	}
	const extent = Math.max(
		...traders.map((trader) => Math.abs(trader.quality)),
		Number.MIN_VALUE,
	);

	return (
		<Flex.Column className="gap-1 border-(--line) border-b p-3">
			{traders.map((trader) => {
				const measured = trader.observed > 0;
				const magnitude = measured
					? share(Math.abs(trader.quality), extent) * 50
					: 0;
				const positive = trader.quality >= 0;

				return (
					<Flex.Row key={trader.id} align="center" gap={2}>
						<Typography.Mono
							size="s"
							tone={trader.id === 0 ? "accent" : "f3"}
							className="w-24 shrink-0 truncate"
						>
							wallet {trader.id + 1}
						</Typography.Mono>
						<div className="relative h-4 flex-1 rounded-[3px] bg-(--sunken) border border-(--line)">
							<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--line2)" />
							{measured ? (
								<div
									className="absolute top-1 bottom-1 rounded-xs"
									style={{
										width: `${magnitude}%`,
										left: positive ? "50%" : undefined,
										right: positive ? undefined : "50%",
										background: positive ? "var(--up)" : "var(--down)",
									}}
								/>
							) : (
								<span className="absolute inset-0 flex items-center justify-center font-mono text-[9px] text-(--f4)">
									nothing graded yet
								</span>
							)}
						</div>
						<Typography.Mono
							size="s"
							tone="f4"
							className="w-28 shrink-0 text-right"
						>
							{measured ? basis(trader.quality) : "—"} · {trader.graded}
						</Typography.Mono>
					</Flex.Row>
				);
			})}
			<Typography.Mono size="s" tone="f4">
				Mean graded outcome per wallet, on one shared axis, with how many
				decisions stand behind each. They hold their own capital and are meant
				to disagree — a wallet consistently to the right of the others has found
				something the rest have not.
			</Typography.Mono>
		</Flex.Column>
	);
};

/*
DrivingActions redraws what the evidence says about each action the agent could
take, as a comparison instead of a table.

The question this answers is "which action does the evidence prefer, and how
much should I believe it" — and that is two quantities per row, not six. The bar
is the measured outcome and which side of the centre it falls on; the strip
beneath it is how much evidence stands behind that, so a confident-looking bar
with nothing under it is visibly different from the same bar with a run of
observations behind it.
*/
export const DrivingActions = ({
	influence,
}: {
	influence: Influence[] | null;
}) => {
	const measured = (influence ?? []).filter((entry) => entry.prior.Defined);

	if (measured.length === 0) {
		return (
			<Flex.Column className="gap-1 border-(--line) border-b p-3">
				<Typography.Mono size="s" tone="f3">
					No action has accumulated evidence yet. A row appears here once a
					decision has been graded, not when one is made.
				</Typography.Mono>
			</Flex.Column>
		);
	}
	const extent = Math.max(
		...measured.map((entry) => Math.abs(entry.prior.Mean)),
		Number.MIN_VALUE,
	);
	const strongest = Math.max(
		...measured.map((entry) => entry.prior.Support),
		Number.MIN_VALUE,
	);

	return (
		<Flex.Column className="gap-2 border-(--line) border-b p-3">
			{measured.map((entry) => {
				const positive = entry.prior.Mean >= 0;
				const magnitude = share(Math.abs(entry.prior.Mean), extent) * 50;

				return (
					<Flex.Column key={`${entry.token}-${entry.action}`} className="gap-1">
						<Flex.Row align="center" gap={2}>
							<Typography.Mono
								size="s"
								tone="accent"
								className="w-28 shrink-0 truncate"
								title={entry.action}
							>
								{entry.action}
							</Typography.Mono>
							<div className="relative h-5 flex-1 rounded-[3px] bg-(--sunken) border border-(--line)">
								<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--line2)" />
								<div
									className="absolute top-1 bottom-1 rounded-xs"
									style={{
										width: `${magnitude}%`,
										left: positive ? "50%" : undefined,
										right: positive ? undefined : "50%",
										background: positive ? "var(--up)" : "var(--down)",
									}}
									title={`${basis(entry.prior.Mean)} over ${entry.prior.Samples} observations`}
								/>
							</div>
							<Typography.Mono
								size="s"
								tone={positive ? "accent" : "f2"}
								className="w-20 shrink-0 text-right"
							>
								{basis(entry.prior.Mean)}
							</Typography.Mono>
						</Flex.Row>
						<Flex.Row align="center" gap={2}>
							<Typography.Mono
								size="s"
								tone="f4"
								className="w-28 shrink-0 truncate"
								title={`${entry.source} / ${entry.label}`}
							>
								{entry.label}
							</Typography.Mono>
							<div className="h-1.5 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
								<div
									className="h-full bg-(--info)"
									style={{
										width: `${share(entry.prior.Support, strongest) * 100}%`,
									}}
									title={`${amount(entry.prior.Support)} effective observations`}
								/>
							</div>
							<Typography.Mono
								size="s"
								tone="f4"
								className="w-20 shrink-0 text-right"
							>
								{entry.prior.Samples.toLocaleString()} obs
							</Typography.Mono>
						</Flex.Row>
					</Flex.Column>
				);
			})}
			<Typography.Mono size="s" tone="f4">
				The bar is what followed this action and which way. The strip under it
				is how much evidence stands behind that — a long bar over a short strip
				is a strong claim on thin ground. Left of centre is against, right is
				for.
			</Typography.Mono>
		</Flex.Column>
	);
};

/*
RehearsalTracks gives each replay worker its own lane showing the tape it is
actually playing back right now: the captured price series of the mounted
fragment, a playhead at the observation it has reached, and one arrow for every
decision it took that was not a wait.

The series is drawn from the delivered observations only. Where the market
coordinate was undefined the line breaks rather than crossing a value that was
never quoted, and a worker with nothing mounted is drawn empty rather than flat
at zero. An arrow is hollow until its decision has been graded; once the tape
has answered it, its colour is the direction of that answer.
*/
export const RehearsalTracks = ({
	tracks,
}: {
	tracks: LearningTrackT[] | null | undefined;
}) => {
	if (!tracks || tracks.length === 0) return null;

	return (
		<Flex.Column className="gap-1">
			{tracks.map((track) => (
				<WorkerTrack key={track.id} track={track} />
			))}
			<Typography.Mono tone="f3">
				The tape runs past a fixed head. B is the observation the move ignited
				at and C the one it ended at; the span between them is the move, shaded
				by what the record says it did. An arrow is a call the worker made,
				coloured by how well it named the moment it reached for.
			</Typography.Mono>
			<Flex className="flex-wrap gap-x-4 gap-y-1">
				{opportunityLegend.map((entry) => (
					<Flex.Row key={entry.label} align="center" gap={1}>
						<span
							className="h-2 w-2 shrink-0 rounded-[2px] opacity-60"
							style={{ background: entry.colour }}
						/>
						<Typography.Mono size="s" tone="f4">
							{entry.label}
						</Typography.Mono>
					</Flex.Row>
				))}
			</Flex>
		</Flex.Column>
	);
};

const WorkerTrack = ({ track }: { track: LearningTrackT }) => {
	const steps = track.steps.filter((step) => step.defined);
	const highest = Math.max(...steps.map((step) => step.value));
	const lowest = Math.min(...steps.map((step) => step.value));
	const span = highest - lowest;

	// An unchanged price is a real reading, not an absent one: it sits on the
	// centre of its own lane instead of being scaled by a zero range.
	const height = (value: number) => (span > 0 ? (value - lowest) / span : 0.5);
	const tall = 40;

	/*
	The tape moves and the playhead stays still, the way a tape actually runs
	past a head. The lane shows a window of observations rather than the whole
	fragment squeezed into its width, so a step is the same distance in every
	lane and nothing jumps when a worker mounts its next tape.

	The window comes from how the fragment was cut: a tape is one lead-in leg,
	the move, and one tail leg, so a third of it is one leg — the amount of
	tape a moment is worth looking at either side of.
	*/
	const window = Math.max(Math.round(track.length / 3), 2);
	const playhead = 0.72;
	const from = track.index - window * playhead;

	/* at places an observation across the lane, rounded so the DOM stays stable. */
	const at = (observation: number) =>
		`${(((observation - from) / window) * 100).toFixed(2)}%`;
	const path = track.steps
		.map((step, index) => {
			if (!step.defined) return "";
			const x = index * track.stride;
			const y = tall - height(step.value) * (tall - 6) - 3;
			const previous = track.steps[index - 1];

			return `${index === 0 || !previous?.defined ? "M" : "L"} ${x} ${y.toFixed(1)}`;
		})
		.filter(Boolean)
		.join(" ");

	return (
		<Flex.Row align="center" gap={2}>
			<Typography.Mono
				size="s"
				tone="f2"
				className="w-32 shrink-0 truncate"
				title={`Worker ${track.id + 1}`}
			>
				↻ {track.id + 1}{" "}
				<span className="text-(--f4)">{String(track.symbol ?? "") || "—"}</span>
			</Typography.Mono>
			<div className="relative h-8 min-w-0 flex-1 overflow-hidden rounded border border-(--line) bg-(--sunken)">
				{track.length === 0 ? (
					<span className="absolute inset-0 flex items-center justify-center font-mono text-[10px] text-(--f4)">
						No fragment mounted
					</span>
				) : (
					<>
						{steps.length > 1 && (
							<svg
								viewBox={`${from} 0 ${window} ${tall}`}
								className="absolute inset-0 h-full w-full select-none"
								preserveAspectRatio="none"
								role="img"
								aria-label={`Tape fragment worker ${track.id + 1} is replaying`}
							>
								<title>{`Tape fragment worker ${track.id + 1} is replaying`}</title>
								<path
									d={path}
									fill="none"
									stroke="var(--f3)"
									strokeWidth="1.5"
									vectorEffect="non-scaling-stroke"
								/>
							</svg>
						)}
						{/* The two moments this tape is asking the worker to recognise. */}
						{track.entry >= 0 && track.exit > track.entry && (
							<>
								{/* The move itself, coloured by what the record says it did. */}
								<div
									className="absolute top-0 bottom-0 opacity-20"
									style={{
										left: at(track.entry),
										width: `${(((track.exit - track.entry) / window) * 100).toFixed(2)}%`,
										background: opportunityColour(
											String(track.opportunity ?? ""),
										),
									}}
								/>
								<Moment
									at={at(track.entry)}
									letter="B"
									label="Ignition: the observation the excursion started at"
								/>
								<Moment
									at={at(track.exit)}
									letter="C"
									label="Extremum: the observation the excursion ended at"
								/>
							</>
						)}
						<div
							className="absolute top-0 bottom-0 w-px bg-(--acc)"
							style={{ left: `${(playhead * 100).toFixed(2)}%` }}
							title={`Observation ${track.index} of ${track.length}`}
						/>
						{track.marks.map((mark) => (
							<span
								key={String(mark.id)}
								className="-translate-x-1/2 -translate-y-1/2 absolute font-mono text-[11px] leading-none"
								style={{
									left: at(mark.index),
									// Arrows are inset inside the lane so a decision taken at
									// the fragment's own extreme is not clipped by its border.
									top: `${((1 - markHeight(track, mark, height)) * 76 + 12).toFixed(2)}%`,
									color: mark.graded
										? mark.value >= 0
											? "var(--up)"
											: "var(--down)"
										: "var(--f2)",
									opacity: mark.graded ? 1 : 0.55,
								}}
								title={`${String(mark.kind)} at observation ${mark.index}${mark.graded ? ` · ${String(mark.verdict ?? "")} · ${mark.value.toFixed(2)}` : " · not judged yet"}`}
							>
								{mark.reduce ? "▼" : "▲"}
							</span>
						))}
					</>
				)}
			</div>
			<Typography.Mono
				size="s"
				tone="f4"
				className="w-28 shrink-0 text-right"
				title={`Observation ${track.index} of ${track.length} · ${track.queued} fragments queued`}
			>
				{track.index}/{track.length} · {track.queued} queued
			</Typography.Mono>
		</Flex.Row>
	);
};

/*
markHeight positions an arrow at the price the decision was actually taken at.
The delivered series is decimated for the socket, so the mark's own observation
index selects the delivered sample it falls on. An undefined coordinate there
leaves the arrow on the centre of the lane rather than at a fabricated price.
*/
const markHeight = (
	track: LearningTrackT,
	mark: LearningMarkT,
	height: (value: number) => number,
) => {
	const step = track.steps[Math.round(mark.index / Math.max(track.stride, 1))];

	return step?.defined ? height(step.value) : 0.5;
};

/* Moment marks one of the two observations a tape is asking a worker to name. */
const Moment = ({
	at,
	letter,
	label,
}: {
	at: string;
	letter: string;
	label: string;
}) => (
	<div
		className="absolute top-0 bottom-0 w-px bg-(--f2)"
		style={{ left: at }}
		title={label}
	>
		<span className="-translate-x-1/2 absolute top-0 left-0 font-mono text-[9px] text-(--f2) leading-none">
			{letter}
		</span>
	</div>
);

/*
opportunityLegend names what the record says a tape did, in the same words and
the same colours the episode pool is counted in beside these lanes. Colour here
is description, not approval: it says what the coordinate did, never that the
worker should have wanted it.
*/
const opportunityLegend = [
	{ label: "rise clears costs", colour: "var(--up)" },
	{ label: "rise eaten by costs", colour: "var(--warn)" },
	{ label: "price falls", colour: "var(--down)" },
	{ label: "exit liquidity unavailable", colour: "var(--warn)" },
	{ label: "no price development", colour: "var(--f3)" },
];

/* opportunityColour leaves an unnamed tape neutral rather than guessing at it. */
const opportunityColour = (opportunity: string) =>
	opportunityLegend.find((entry) => entry.label === opportunity)?.colour ??
	"var(--f3)";
