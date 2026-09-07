import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { amount, basis, clock, duration, percent } from "./format";
import type {
	AccountLearning,
	ForwardReview,
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
	const execution = view?.execution;
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
			note: "Nothing is sent while the agent is still calibrating",
		},
		...(view?.hasExecution
			? [
					{
						key: "submitted",
						label: "Orders the account placed",
						count: execution?.submitted ?? 0,
						note: `${execution?.dropped ?? 0} went stale · ${execution?.failed ?? 0} refused · ${execution?.diverged ?? 0} disagreed`,
					},
				]
			: []),
		{
			key: "resolved",
			label: "Outcomes measured",
			count: view?.resolved ?? 0,
			note: "A decision becomes evidence only once its window closes",
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
										key={`${entry.event.lane}-${entry.event.id}-${entry.at}`}
										className="absolute top-[3px] bottom-[3px] w-px"
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

/*
PromotionLadder is the promotion rule drawn as one picture: the measured range
of the edge, and the line it has to clear before the agent is allowed to trade.

The bar is the conservative bound, not the mean. That is deliberate — the rule
reads the bound, so the picture reads the bound, and an operator watching this
shape sees exactly the quantity the machine is waiting on.
*/
export const PromotionLadder = ({ skill }: { skill?: Skill }) => {
	if (!skill?.defined) {
		return (
			<Flex.Column className="gap-1 border-(--line) border-b p-3">
				<Typography.Label size="s" tone="f4" weight="normal">
					Distance to trading
				</Typography.Label>
				<Typography.Mono size="s" tone="f3">
					No resolved evidence yet — there is nothing to measure a distance
					against.
				</Typography.Mono>
			</Flex.Column>
		);
	}

	const boundBp = skill.lowerBound * 10000;
	const meanBp = skill.mean * 10000;
	const extent = Math.max(Math.abs(boundBp), Math.abs(meanBp), 1) * 1.2;
	const place = (valueBp: number) => 50 + (valueBp / extent) * 50;
	const cleared = skill.qualified && skill.lowerBound > 0;

	return (
		<Flex.Column className="gap-2 border-(--line) border-b p-3">
			<Typography.Label size="s" tone="f4" weight="normal">
				Distance to trading
			</Typography.Label>
			<div className="relative h-8 w-full rounded bg-(--sunken) border border-(--line)">
				<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--line2)" />
				{/* The measured range: conservative bound up to the mean. */}
				<div
					className="absolute top-2 bottom-2 rounded-xs opacity-40"
					style={{
						left: `${Math.min(place(boundBp), place(meanBp))}%`,
						width: `${Math.abs(place(meanBp) - place(boundBp))}%`,
						background: cleared ? "var(--up)" : "var(--info)",
					}}
				/>
				{/* The bound itself, which is what the promotion rule reads. */}
				<div
					className="absolute top-1 bottom-1 w-[2px]"
					style={{
						left: `${place(boundBp)}%`,
						background: cleared ? "var(--up)" : "var(--warn)",
					}}
					title={`Conservative bound ${basis(skill.lowerBound)} at ${skill.sigma}σ`}
				/>
			</div>
			<Flex.Row className="justify-between font-mono text-[9px] text-(--f4)">
				<span>worse</span>
				<span>the line it must cross</span>
				<span>better</span>
			</Flex.Row>
			<Typography.Mono size="s" tone={cleared ? "accent" : "f3"}>
				{cleared
					? `Cleared by ${basis(skill.lowerBound)} — the worst credible reading is still a profit.`
					: `Short of the line by ${basis(Math.abs(skill.lowerBound))} — the worst credible reading is still a loss.`}
			</Typography.Mono>
			<Flex.Column className="gap-1 pt-1">
				<Flex.Row className="justify-between font-mono text-[9px] text-(--f4)">
					<span>Evidence actually independent</span>
					<span>
						{skill.support.toFixed(1)} of {skill.samples.toLocaleString()}
					</span>
				</Flex.Row>
				<div className="h-1.5 w-full overflow-hidden rounded-[3px] bg-(--line)">
					<div
						className="h-full bg-(--info)"
						style={{ width: `${share(skill.support, skill.samples) * 100}%` }}
					/>
				</div>
			</Flex.Column>
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
InfluenceGrid is the discovery table as a matrix: one row per measured quantity,
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
								title={`${row.source} / ${row.label} · ${label}\nMean outcome ${basis(cell.prior.Mean)}/s\nAuthority ${percent(cell.prior.Authority)}\n${cell.prior.Samples} samples`}
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
									className="absolute bottom-0 left-0 h-[3px] bg-(--f1) opacity-70"
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

/*
ExposureRing shows the forward test as one shape: of the excursions the tape
actually offered, how many the policy was in for. "Unexposed" is not an error —
the excursion only became visible after the decision was due — so it is drawn
as a neutral slice, not a red one.
*/
export const ExposureRing = ({ forward }: { forward?: ForwardReview }) => {
	const exposed = forward?.exposed ?? forward?.captured ?? 0;
	const unexposed = forward?.unexposed ?? forward?.missed ?? 0;
	const unknown = forward?.unreviewable ?? 0;
	const total = exposed + unexposed + unknown;

	const slices = [
		{
			key: "exposed",
			label: "held through it",
			value: exposed,
			tone: "var(--up)",
		},
		{
			key: "unexposed",
			label: "sat it out",
			value: unexposed,
			tone: "var(--warn)",
		},
		{
			key: "unknown",
			label: "not reviewable",
			value: unknown,
			tone: "var(--f4)",
		},
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
				aria-label="Confirmed excursions the policy was exposed to"
			>
				<title>Confirmed excursions the policy was exposed to</title>
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
					{total > 0 ? percent(exposed / total) : "—"}
				</text>
				<text
					x="70"
					y="84"
					textAnchor="middle"
					fill="var(--f4)"
					fontSize="9"
					fontFamily="monospace"
				>
					in for it
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
					Of the {total.toLocaleString()} moves the tape confirmed after the
					fact, this is how many the policy happened to be holding through.
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};

/*
WalletBars puts every cloned wallet on one axis so the spread between them is
visible without reading a column of figures. A wallet that has not been valued
yet is drawn as a gap on the axis rather than as a bar at zero.
*/
export const WalletBars = ({ lanes }: { lanes: Wallet[] | null }) => {
	const wallets = lanes ?? [];
	if (wallets.length === 0) return null;

	const extent = Math.max(
		...wallets.map((wallet) => (wallet.complete ? Math.abs(wallet.profit) : 0)),
		...wallets.map((wallet) => Math.abs(wallet.realized)),
		1e-9,
	);

	return (
		<Flex.Column className="gap-1 border-(--line) border-b p-3">
			{wallets.map((wallet) => {
				const value = wallet.complete ? wallet.profit : undefined;
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
							{wallet.exhausted ? " · spent" : ""}
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
				This episode's profit for each wallet, on one shared axis. Each wallet
				owns its own capital; a spent one restarts as a fresh clone rather than
				carrying a balance forward.
			</Typography.Mono>
		</Flex.Column>
	);
};

/*
ExcursionBar draws how far an allocation ran in each direction before it was
measured: the worst it looked, the best it looked, and where it actually
finished. Three numbers that only mean something in relation to each other, so
they are shown in relation to each other.
*/
export const ExcursionBar = ({ account }: { account: AccountLearning }) => {
	const best = Math.max(account.mfe, 0);
	const worst = Math.min(account.mae, 0);
	const extent = Math.max(best, Math.abs(worst), 1e-9);
	const place = (value: number) => 50 + (value / extent) * 50;
	const finished = account.state.mark.version
		? account.outcome.totalReward
		: undefined;

	if (best === 0 && worst === 0) return null;

	return (
		<Flex.Column className="gap-1">
			<div className="relative h-7 w-full rounded bg-(--sunken) border border-(--line)">
				<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--line2)" />
				<div
					className="absolute top-2 bottom-2 rounded-xs opacity-30 bg-(--down)"
					style={{ left: `${place(worst)}%`, width: `${50 - place(worst)}%` }}
					title={`Worst it looked: ${amount(worst)}`}
				/>
				<div
					className="absolute top-2 bottom-2 rounded-xs opacity-30 bg-(--up)"
					style={{ left: "50%", width: `${place(best) - 50}%` }}
					title={`Best it looked: ${amount(best)}`}
				/>
				{finished !== undefined && (
					<div
						className="absolute top-0.5 bottom-0.5 w-[2px] bg-(--f1)"
						style={{ left: `${place(finished)}%` }}
						title={`Where it finished: ${amount(finished)}`}
					/>
				)}
			</div>
			<Flex.Row className="justify-between font-mono text-[9px] text-(--f4)">
				<span>worst {amount(worst)}</span>
				<span>
					{finished === undefined
						? "no authoritative mark"
						: `finished ${amount(finished)}`}
				</span>
				<span>best {amount(best)}</span>
			</Flex.Row>
			{account.timeToPositiveNs > 0 && (
				<Typography.Mono size="s" tone="f4">
					First went positive after {duration(account.timeToPositiveNs)} · held{" "}
					{duration(account.holdingNs)}
				</Typography.Mono>
			)}
		</Flex.Column>
	);
};

/*
MeasurementWindow explains the one number that decides whether this market can
be learned from at all: how long the agent waits before scoring a decision.

It is not a setting. It is a race between two measured quantities — how far the
price moves on its own, and how much it costs to get in and out — and the
window is the point where the first can plausibly cover the second. Drawing
them against each other says why the wait is what it is, so a window of four
seconds and a window of four minutes read as facts about two different markets
rather than as a knob someone turned.
*/
export const MeasurementWindow = ({ view }: { view: LearningView | null }) => {
	const cost = view?.roundTrip ?? 0;
	const movement = view?.movement ?? 0;
	const measured = (view?.hasMovement ?? false) && cost > 0;

	if (!measured) {
		return (
			<Flex.Column className="gap-1 border-(--line) border-b p-3">
				<Typography.Label size="s" tone="f4" weight="normal">
					Measurement window
				</Typography.Label>
				<Typography.Mono size="s" tone="f3">
					Not measurable yet — this market's own movement has not been observed
					often enough to say how long an outcome needs. Until it has, nothing
					here is scored: a window that was guessed at would answer every
					decision with the spread it just paid.
				</Typography.Mono>
			</Flex.Column>
		);
	}

	const extent = Math.max(cost, movement);
	const observations = Math.max(1, Math.round(view?.horizonObservations ?? 0));

	return (
		<Flex.Column className="gap-2 border-(--line) border-b p-3">
			<Flex.Row align="center" className="justify-between">
				<Typography.Label size="s" tone="f4" weight="normal">
					Measurement window
				</Typography.Label>
				<Typography.Mono
					size="s"
					tone={view?.horizonCapped ? "down" : "accent"}
				>
					{duration(view?.horizonNs ?? 0)}
					{view?.horizonCapped ? " · at the ceiling" : ""}
				</Typography.Mono>
			</Flex.Row>

			<Flex.Column className="gap-1">
				<Flex.Row align="center" gap={2}>
					<Typography.Mono size="s" tone="f3" className="w-40 shrink-0">
						costs to round-trip
					</Typography.Mono>
					<div className="h-3 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
						<div
							className="h-full bg-(--down)"
							style={{ width: `${share(cost, extent) * 100}%` }}
						/>
					</div>
					<Typography.Mono
						size="s"
						tone="f4"
						className="w-20 shrink-0 text-right"
					>
						{percent(cost)}
					</Typography.Mono>
				</Flex.Row>
				<Flex.Row align="center" gap={2}>
					<Typography.Mono size="s" tone="f3" className="w-40 shrink-0">
						moves per observation
					</Typography.Mono>
					<div className="h-3 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
						<div
							className="h-full bg-(--up)"
							style={{ width: `${share(movement, extent) * 100}%` }}
						/>
					</div>
					<Typography.Mono
						size="s"
						tone="f4"
						className="w-20 shrink-0 text-right"
					>
						{percent(movement)}
					</Typography.Mono>
				</Flex.Row>
			</Flex.Column>

			<Typography.Mono size="s" tone="f4">
				{view?.horizonCapped
					? `This market moves ${percent(movement)} at a time and costs ${percent(cost)} to get in and out, so covering that cost takes longer than an outcome can still be credited to the decision that opened it. It is reporting that it cannot be traded profitably at this size.`
					: `This market moves ${percent(movement)} at a time and costs ${percent(cost)} to get in and out, so it takes about ${observations.toLocaleString()} observations — ${duration(view?.horizonNs ?? 0)} — before a move is big enough to say whether the decision was any good.`}
			</Typography.Mono>
		</Flex.Column>
	);
};
