import { useMemo, useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import type { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";

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
*/

export type Mark = {
	sequence: number;
	label: string;
};

/* One comparable fact, addressed by a stable identity across marks. */
type Fact = {
	id: string;
	group: string;
	name: string;
	/* One entry per mark: a number, null for undefined, undefined for absent. */
	values: Array<number | null | undefined>;
	unit: string;
};

const FACT_GROUPS = ["measurement", "category", "perspective"] as const;

/*
readFacts flattens one decoded state into the named facts a comparison can line
up: every signal metric by "source/metric", every category by its confidence,
and every perspective reading by "symbol/metric".
*/
const readFacts = (state: EnvelopeState | null): Map<string, { group: string; name: string; unit: string; value: number | null }> => {
	const facts = new Map<
		string,
		{ group: string; name: string; unit: string; value: number | null }
	>();

	if (state === null) return facts;

	const measurements = [
		{ source: "cvd", value: state.cvd() },
		{ source: "hawkes", value: state.hawkes() },
		{ source: "depthFlow", value: state.depthFlow() },
		{ source: "morphology", value: state.morphology() },
		{ source: "liquidity", value: state.liquidity() },
		{ source: "correlation", value: state.correlation() },
		{ source: "leadLag", value: state.leadLag() },
		{ source: "sentiment", value: state.sentiment() },
		{ source: "pumpDump", value: state.pumpDump() },
		{ source: "toxicity", value: state.toxicity() },
		{ source: "derivatives", value: state.derivatives() },
	];

	for (const { source, value } of measurements) {
		if (value === null) continue;

		for (let index = 0; index < value.metricsLength(); index++) {
			const entry = value.metrics(index);
			const metric = entry?.value();

			if (entry === null || metric === null || metric === undefined) continue;

			const name = `${source}/${entry.key() ?? ""}`;

			facts.set(name, {
				group: "measurement",
				name,
				unit: metric.unit() ?? "",
				value: Number.isFinite(metric.raw()) ? metric.raw() : null,
			});
		}
	}

	for (let index = 0; index < state.categoriesLength(); index++) {
		const category = state.categories(index);

		if (category === null) continue;

		const name = `${category.type() ?? ""}`;

		facts.set(`category/${name}`, {
			group: "category",
			name: `${name} · confidence`,
			unit: "",
			value: category.confidence(),
		});
	}

	for (let index = 0; index < state.perspectivesLength(); index++) {
		const perspective = state.perspectives(index);

		if (perspective === null) continue;

		const symbol = perspective.symbol() ?? "";

		for (let reading = 0; reading < perspective.readingsLength(); reading++) {
			const entry = perspective.readings(reading);

			if (entry === null) continue;

			const metric = entry.metric() ?? "";
			const name = `${symbol}/${metric}`;

			facts.set(`perspective/${name}`, {
				group: "perspective",
				name,
				unit: "",
				value: entry.defined() ? entry.value() : null,
			});
		}
	}

	return facts;
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
	onPlayhead,
	onClear,
	onRemove,
}: {
	marks: Mark[];
	states: Array<EnvelopeState | null>;
	onPlayhead: (sequence: number) => void;
	onClear: () => void;
	onRemove: (sequence: number) => void;
}) => {
	const [onlyChanged, setOnlyChanged] = useState(true);
	const [group, setGroup] = useState<string | null>(null);
	const [filter, setFilter] = useState("");

	const facts = useMemo(() => {
		const perMark = states.map(readFacts);
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
			});
		}

		rows.sort((left, right) => {
			if (left.group !== right.group) return left.group < right.group ? -1 : 1;

			return left.name < right.name ? -1 : 1;
		});

		return rows;
	}, [states]);

	const rows = useMemo(() => {
		const needle = filter.trim().toLowerCase();

		return facts.filter((fact) => {
			if (group !== null && fact.group !== group) return false;
			if (needle !== "" && !fact.name.toLowerCase().includes(needle)) return false;
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
						{moved} of {facts.length} facts changed
					</span>
				}
			/>

			<div className="shrink-0 border-(--line) border-b px-2.5 py-1.5">
				<Flex.Row gap={2} className="flex-wrap items-center">
					{marks.map((mark, index) => (
						<Flex.Row
							key={mark.sequence}
							align="center"
							className="rounded-[3px] border border-(--info) px-1"
						>
							<Button
								variant="bare"
								className="font-mono text-[9px] text-(--f1)"
								title="Park the playhead back on this mark."
								onClick={() => onPlayhead(mark.sequence)}
							>
								<span className="text-(--info)">
									{String.fromCharCode(65 + index)}
								</span>{" "}
								#{mark.sequence}
							</Button>
							<Button
								variant="bare"
								className="pl-1 font-mono text-[9px] text-(--f4) hover:text-(--down)"
								title="Drop this mark."
								onClick={() => onRemove(mark.sequence)}
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

				<Flex.Row gap={2} className="mt-1.5 flex-wrap items-center">
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
							className={`rounded-[2px] border px-1 font-mono text-[9px] ${
								group === option
									? "border-(--acc) text-(--f1)"
									: "border-(--line) text-(--f4) hover:text-(--f2)"
							}`}
							onClick={() => setGroup((current) => (current === option ? null : option))}
						>
							{option}
						</Button>
					))}
					<input
						value={filter}
						placeholder="filter fact"
						spellCheck={false}
						className="ml-auto w-40 rounded-[2px] border border-(--line) bg-(--sunken) px-1 py-0.5 font-mono text-[9px] text-(--f1) outline-none focus:border-(--line2)"
						onChange={(event) => setFilter(event.currentTarget.value)}
					/>
				</Flex.Row>
			</div>

			<Section.Body>
				{rows.length === 0 ? (
					<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
						{facts.length === 0
							? "No state was witnessed at these marks. Unavailable — not unchanged."
							: "Nothing changed between these marks under the current filter."}
					</p>
				) : (
					<table className="w-full border-collapse font-mono text-[9px]">
						<thead className="sticky top-0 bg-(--surface)">
							<tr className="text-left text-(--f4)">
								<th className="px-2.5 py-1 font-normal">fact</th>
								{marks.map((mark, index) => (
									<th key={mark.sequence} className="px-2 py-1 text-right font-normal">
										<span className="text-(--info)">
											{String.fromCharCode(65 + index)}
										</span>{" "}
										#{mark.sequence}
									</th>
								))}
								<th className="px-2 py-1 font-normal">unit</th>
							</tr>
						</thead>
						<tbody>
							{rows.map((fact) => (
								<tr key={fact.id} className="border-(--line) border-t align-baseline">
									<td className="px-2.5 py-0.5 text-(--f2)">
										<span className="text-(--f4)">{fact.group.slice(0, 4)} </span>
										{fact.name}
									</td>
									{fact.values.map((value, index) => {
										const previous = index === 0 ? undefined : fact.values[index - 1];
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
												key={`${fact.id}-${marks[index]?.sequence ?? index}`}
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
											</td>
										);
									})}
									<td className="px-2 py-0.5 text-(--f4)">{fact.unit || "—"}</td>
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
				<span key={mark.sequence} className="text-(--info)">
					{String.fromCharCode(65 + index)}
					<span className="text-(--f4)"> #{mark.sequence}</span>
					{index < marks.length - 1 ? " →" : ""}
				</span>
			))
		)}
		<span className="text-(--f4)">
			{marks.length > 0 && marks.length < 2 ? "· mark another to compare" : ""}
		</span>
	</Flex.Row>
);
