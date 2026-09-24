import { useMemo, useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import type { Measurement } from "./hindsight-types";
/*
Comparing what SYMM held at two or three exact capture coordinates.

This is the question a position post-mortem actually asks: not "what was the
state", but "what changed between the moment before entry, the moment of entry,
and the moment of exit". So the view is a fact table — one row per named fact,
one column per mark — and the rows that did not move can be hidden, because a
hundred unchanged numbers are what hides the four that moved.

Three states are kept apart everywhere here, and never merged:

    a value that changed        the number at each mark
    a value that was undefined  the estimator could not estimate it
    a fact that was absent      the mark's state carried no such fact at all

Collapsing "absent" into "0" would manufacture a delta out of nothing, which is
precisely the reasoning error this whole surface exists to prevent.

There are two honest answers to "what was the state here", and they are offered
as explicit modes rather than blended:

    exact capture     what this one envelope produced and carried
    resident as-of    the latest value causally available at this coordinate,
                      for every signal family, however long ago it was produced

Exact capture is the mode for provenance: it shows what this frame did. But a
signal is only recomputed on the envelopes that feed it, so in that mode a
family reads as absent on every frame that did not touch it — which looks
exactly like "SYMM did not know this" and is not. Resident as-of answers the
question a post-mortem actually asks, and every carried value shows the capture
it came from and how old it was, so the difference stays visible.
*/

export type CompareMode = "exact" | "resident";

export type Mark = {
	sequence: number;
	ordinal: number;
	label: string;
};

/* One comparable fact, addressed by a stable identity across marks. */
type Fact = {
	id: string;
	group: string;
	name: string;
	/* One entry per mark: a number, null for undefined, undefined for absent. */
	values: Array<number | null | undefined>;
	/* Where each value came from, in resident mode. */
	origins: Array<{
		sequence: number;
		ordinal: number;
		ageMs: number | null;
		carried: boolean;
	} | null>;
	unit: string;
};

type Reading = {
	group: string;
	name: string;
	unit: string;
	value: number | null;
	origin: {
		sequence: number;
		ordinal: number;
		ageMs: number | null;
		carried: boolean;
	} | null;
};

const readMeasurement = (m: Measurement | null): Map<string, Reading> => {
	const facts = new Map<string, Reading>();
	if (m === null || m === undefined) return facts;

	const all = [m, ...(m.peers ?? m.Peers ?? [])];
	for (const item of all) {
		const source = item.source || item.label || "measurement";
		const seq =
			typeof item.seqIdx === "number" ? item.seqIdx : Number(item.seqIdx) || 0;
		const origin = {
			sequence: seq,
			ordinal: 0,
			ageMs: null,
			carried: false,
		};

		if (item.maturity !== undefined) {
			facts.set(`${source}/maturity`, {
				group: "measurement",
				name: `${source}/maturity`,
				unit: "",
				value: item.maturity,
				origin,
			});
		}

		if (item.snrDefined && item.snr !== undefined) {
			facts.set(`${source}/snr`, {
				group: "measurement",
				name: `${source}/snr`,
				unit: "dB",
				value: item.snr,
				origin,
			});
		}

		for (const [key, metric] of Object.entries(item.metrics ?? {})) {
			const name = `${source}/${key}`;
			facts.set(name, {
				group: "measurement",
				name,
				unit: String(metric.unit ?? ""),
				value: Number.isFinite(metric.raw) ? metric.raw : null,
				origin,
			});
		}
	}

	return facts;
};

const FACT_GROUPS = ["measurement", "category", "legacy-advisor"] as const;

/*
readFacts flattens one decoded state into the named facts a comparison can line
up: every signal metric by "source/metric", every category by its confidence,
and every retired advisor reading by "symbol/metric".
*/
const readFacts = (_state: unknown): Map<string, Reading> => {
	return new Map<string, Reading>();
};

const changed = (values: Array<number | null | undefined>): boolean => {
	const seen = values.map((value) =>
		value === undefined ? "absent" : value === null ? "undefined" : value,
	);

	return seen.some((value) => value !== seen[0]);
};

const formatCell = (value: number | null | undefined): string => {
	if (value === undefined) return "absent";
	if (value === null) return "undef";
	if (!Number.isFinite(value)) return String(value);
	if (value === 0) return "0";

	const magnitude = Math.abs(value);

	if (magnitude >= 1e6 || magnitude < 1e-4) return value.toExponential(3);
	if (magnitude >= 100) return value.toFixed(2);
	if (magnitude >= 1) return value.toFixed(4);

	return value.toFixed(6);
};

export const ComparePanel = ({
	marks,
	states,
	measurements,
	mode,
	loading,
	onMode,
	onPlayhead,
	onClear,
	onRemove,
}: {
	marks: Mark[];
	states?: Array<unknown>;
	measurements?: Array<Measurement | null>;
	mode: CompareMode;
	loading: boolean;
	onMode: (next: CompareMode) => void;
	onPlayhead: (sequence: number, ordinal: number) => void;
	onClear: () => void;
	onRemove: (sequence: number, ordinal: number) => void;
}) => {
	const [onlyChanged, setOnlyChanged] = useState(true);
	const [group, setGroup] = useState<string | null>(null);
	const [filter, setFilter] = useState("");

	const facts = useMemo(() => {
		const perMark =
			measurements && measurements.length > 0
				? measurements.map(readMeasurement)
				: (states ?? []).map(readFacts);
		const identities = new Set<string>();

		for (const mark of perMark) {
			for (const id of mark.keys()) identities.add(id);
		}

		const rows: Fact[] = [];

		for (const id of identities) {
			const present = perMark.find((mark) => mark.has(id))?.get(id);

			if (present === undefined) continue;

			rows.push({
				id,
				group: present.group,
				name: present.name,
				unit: present.unit,
				values: perMark.map((mark) => {
					const fact = mark.get(id);

					return fact === undefined ? undefined : fact.value;
				}),
				origins: perMark.map((mark) => mark.get(id)?.origin ?? null),
			});
		}

		rows.sort((left, right) => {
			if (left.group !== right.group) return left.group < right.group ? -1 : 1;

			return left.name < right.name ? -1 : 1;
		});

		return rows;
	}, [states, measurements]);

	const rows = useMemo(() => {
		const needle = filter.trim().toLowerCase();

		return facts.filter((fact) => {
			if (group !== null && fact.group !== group) return false;
			if (needle !== "" && !fact.name.toLowerCase().includes(needle))
				return false;
			if (onlyChanged && !changed(fact.values)) return false;

			return true;
		});
	}, [facts, group, filter, onlyChanged]);

	const moved = facts.filter((fact) => changed(fact.values)).length;

	return (
		<Section fit="pane" surface="surface" className="min-h-0 flex-1">
			<Section.Header
				title="Compare marks"
				size="m"
				rule
				sticky
				meta={
					<span className="font-mono text-[9px] text-(--f4)">
						{loading
							? "resolving…"
							: `${moved} of ${facts.length} facts changed`}
					</span>
				}
			/>

			<Flex.Row
				align="center"
				gap={2}
				className="shrink-0 flex-wrap border-(--line) border-b bg-(--sunken) px-2.5 py-1.5 font-mono text-[8px] text-(--f4)"
			>
				<span className="uppercase tracking-widest">state</span>
				<Button
					variant="bare"
					title="What this exact envelope produced and carried. A family absent here was not recomputed on this frame — which is not the same as the system not holding it."
					className={`rounded-xs border px-1 py-0.5 font-mono text-[9px] ${
						mode === "exact"
							? "border-(--acc) text-(--f1)"
							: "border-(--line) text-(--f4) hover:text-(--f2)"
					}`}
					onClick={() => onMode("exact")}
				>
					exact capture
				</Button>
				<Button
					variant="bare"
					title="The latest value causally available at this coordinate for every signal family, however long ago it was produced. Resolved by capture order, never by nearest timestamp."
					className={`rounded-xs border px-1 py-0.5 font-mono text-[9px] ${
						mode === "resident"
							? "border-(--acc) text-(--f1)"
							: "border-(--line) text-(--f4) hover:text-(--f2)"
					}`}
					onClick={() => onMode("resident")}
				>
					resident as-of
				</Button>

				<span className="ml-2">
					{marks.map((mark, index) => {
						const m = measurements?.[index];
						const peers = m ? (m.peers ?? m.Peers ?? []) : [];

						return (
							<span key={`${mark.sequence}:${mark.ordinal}`} className="mr-3">
								<span className="text-(--info)">
									{String.fromCharCode(65 + index)}
								</span>{" "}
								seq {mark.sequence}
								{m?.source ? ` · ${m.source}` : ""}
								{peers.length > 0 ? ` (${peers.length} peers)` : ""}
							</span>
						);
					})}
				</span>
			</Flex.Row>

			<div className="shrink-0 border-(--line) border-b px-2.5 py-1.5">
				<Flex.Row align="center" gap={2} className="flex-wrap">
					{marks.map((mark, index) => (
						<Flex.Row
							key={`${mark.sequence}:${mark.ordinal}`}
							align="center"
							className="rounded-[3px] border border-(--info) px-1"
						>
							<Button
								variant="bare"
								className="font-mono text-[9px] text-(--f1)"
								title="Park the playhead back on this mark."
								onClick={() => onPlayhead(mark.sequence, mark.ordinal)}
							>
								<span className="text-(--info)">
									{String.fromCharCode(65 + index)}
								</span>{" "}
								#{mark.sequence}:{mark.ordinal}
							</Button>
							<Button
								variant="bare"
								className="pl-1 font-mono text-[9px] text-(--f4) hover:text-(--down)"
								title="Drop this mark."
								onClick={() => onRemove(mark.sequence, mark.ordinal)}
							>
								×
							</Button>
						</Flex.Row>
					))}
					<Button
						variant="bare"
						size="xs"
						className="font-mono text-[9px] text-(--f4) hover:text-(--f1)"
						onClick={onClear}
					>
						clear all
					</Button>
				</Flex.Row>

				<Flex.Row align="center" gap={2} className="mt-1.5 flex-wrap">
					<Button
						variant="bare"
						className="font-mono text-[9px] text-(--f4) hover:text-(--f2)"
						onClick={() => setOnlyChanged((current) => !current)}
					>
						<span className={onlyChanged ? "text-(--acc)" : ""}>
							{onlyChanged ? "▣" : "▢"}
						</span>{" "}
						only what changed
					</Button>
					{FACT_GROUPS.map((option) => (
						<Button
							key={option}
							variant="bare"
							className={`rounded-xs border px-1 font-mono text-[9px] ${
								group === option
									? "border-(--acc) text-(--f1)"
									: "border-(--line) text-(--f4) hover:text-(--f2)"
							}`}
							onClick={() =>
								setGroup((current) => (current === option ? null : option))
							}
						>
							{option}
						</Button>
					))}
					<input
						value={filter}
						placeholder="filter fact"
						spellCheck={false}
						className="ml-auto w-40 rounded-xs border border-(--line) bg-(--sunken) px-1 py-0.5 font-mono text-[9px] text-(--f1) outline-none focus:border-(--line2)"
						onChange={(event) => setFilter(event.currentTarget.value)}
					/>
				</Flex.Row>
			</div>

			<Section.Body>
				{rows.length === 0 ? (
					<Typography.Paragraph className="px-3 py-3 font-mono text-[10px] text-(--f4)">
						{facts.length === 0
							? "No state was witnessed at these marks. Unavailable — not unchanged."
							: "Nothing changed between these marks under the current filter."}
					</Typography.Paragraph>
				) : (
					<table className="w-full border-collapse font-mono text-[9px]">
						<thead className="sticky top-0 bg-(--surface)">
							<tr className="text-left text-(--f4)">
								<th className="px-2.5 py-1 font-normal">fact</th>
								{marks.map((mark, index) => (
									<th
										key={`${mark.sequence}:${mark.ordinal}`}
										className="px-2 py-1 text-right font-normal"
									>
										<span className="text-(--info)">
											{String.fromCharCode(65 + index)}
										</span>{" "}
										#{mark.sequence}:{mark.ordinal}
									</th>
								))}
								<th className="px-2 py-1 font-normal">unit</th>
							</tr>
						</thead>
						<tbody>
							{rows.map((fact) => (
								<tr
									key={fact.id}
									className="border-(--line) border-t align-baseline"
								>
									<td className="px-2.5 py-0.5 text-(--f2)">
										<span className="text-(--f4)">
											{fact.group.slice(0, 4)}{" "}
										</span>
										{fact.name}
									</td>
									{fact.values.map((value, index) => {
										const origin = fact.origins[index];
										const previous =
											index === 0 ? undefined : fact.values[index - 1];
										const moves =
											index > 0 &&
											(value === undefined || previous === undefined
												? value !== previous
												: value === null || previous === null
													? value !== previous
													: value !== previous);
										const direction =
											typeof value === "number" && typeof previous === "number"
												? value - previous
												: 0;

										return (
											<td
												key={`${fact.id}-${marks[index]?.sequence ?? index}:${marks[index]?.ordinal ?? 0}`}
												className={`px-2 py-0.5 text-right tabular-nums ${
													value === undefined
														? "text-(--f4)"
														: value === null
															? "text-(--warn)"
															: moves
																? direction > 0
																	? "text-(--up)"
																	: direction < 0
																		? "text-(--down)"
																		: "text-(--f1)"
																: "text-(--f3)"
												}`}
											>
												{formatCell(value)}
												{origin === null ? null : (
													<div
														className={`text-[7.5px] ${origin.carried ? "text-(--warn)" : "text-(--f4)"}`}
														title={`Resolved from capture #${origin.sequence}:${origin.ordinal}${
															origin.carried
																? " — carried from an earlier envelope, not produced at this mark"
																: " — produced at this mark"
														}`}
													>
														#{origin.sequence}:{origin.ordinal}
														{origin.ageMs === null
															? ""
															: ` · ${
																	origin.ageMs < 1000
																		? `${Math.round(origin.ageMs)}ms`
																		: `${(origin.ageMs / 1000).toFixed(1)}s`
																} old`}
													</div>
												)}
											</td>
										);
									})}
									<td className="px-2 py-0.5 text-(--f4)">
										{fact.unit || "—"}
									</td>
								</tr>
							))}
						</tbody>
					</table>
				)}
			</Section.Body>
		</Section>
	);
};

/*
MarkBar is the always-present control for placing marks. It is deliberately not
hidden behind the compare view: the moment worth marking is usually noticed
while looking at something else.
*/
export const MarkBar = ({
	marks,
	playhead,
	onMark,
}: {
	marks: Mark[];
	playhead: number | null;
	onMark: () => void;
}) => (
	<Flex.Row align="center" gap={2} className="font-mono text-[9px] text-(--f4)">
		<Button
			variant="outline"
			size="xs"
			className="font-mono text-[9px]"
			disabled={playhead === null || marks.length >= 3}
			title="Mark the frame under the playhead for comparison (m). Up to three."
			onClick={onMark}
		>
			mark (m)
		</Button>
		{marks.length === 0 ? (
			<span>no marks</span>
		) : (
			marks.map((mark, index) => (
				<span
					key={`${mark.sequence}:${mark.ordinal}`}
					className="text-(--info)"
				>
					{String.fromCharCode(65 + index)}
					<span className="text-(--f4)">
						{" "}
						#{mark.sequence}:{mark.ordinal}
					</span>
					{index < marks.length - 1 ? " →" : ""}
				</span>
			))
		)}
		<span className="text-(--f4)">
			{marks.length > 0 && marks.length < 2 ? "· mark another to compare" : ""}
		</span>
	</Flex.Row>
);
