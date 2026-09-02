import * as flatbuffers from "flatbuffers";
import { Fragment, useMemo, useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import type { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import type {
	HindsightCapture,
	HindsightEnvelope,
	HindsightMetricMap,
	HindsightResident,
	HindsightRun,
	MetricSemantics,
} from "./hindsight-types";
import { formatClock } from "./timeline-scale";

/*
The inspection view for one ReferencePoint.

It reads what the running binary actually produced at that exact boundary, and
it stays navigable by identity: an artifact names its parent envelopes, and
those are clickable, so a finding can be walked back to the raw frame without a
single timestamp comparison anywhere in the path.

Nothing here recomputes an artifact. If the live code emitted an incorrect
value, this shows the incorrect value. That is the evidence.
*/

export const decodeEnvelopeState = (payload: unknown): EnvelopeState | null => {
	if (typeof payload !== "string" || payload === "") {
		return null;
	}

	try {
		const bytes = Uint8Array.from(atob(payload), (char) => char.charCodeAt(0));

		return EnvelopeState.getRootAsEnvelopeState(
			new flatbuffers.ByteBuffer(bytes),
		);
	} catch {
		return null;
	}
};

const Field = ({ label, value }: { label: string; value: React.ReactNode }) => (
	<Flex.Column className="min-w-0">
		<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
			{label}
		</span>
		<span className="truncate font-mono text-[10px] text-(--f1)">{value}</span>
	</Flex.Column>
);

/*
CaptureCard states the identity of the frame under the playhead. Every field is
part of the identity assigned before the bytes were parsed, which is what makes
this frame distinguishable from another one carrying the same venue timestamp.
*/
export const CaptureCard = ({
	capture,
	run,
}: {
	capture: HindsightCapture | null;
	run: HindsightRun | null;
}) => {
	if (capture === null) {
		return (
			<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
				Park the playhead on the timeline to inspect an exact captured frame.
			</p>
		);
	}

	return (
		<div className="grid grid-cols-4 gap-x-4 gap-y-2 border-(--line) border-b px-3 py-2.5">
			<Field
				label="capture"
				value={
					<span className="tabular-nums">#{capture.identity.sequence}</span>
				}
			/>
			<Field label="kind" value={capture.kind || "—"} />
			<Field
				label="stream epoch"
				value={
					<span className="tabular-nums">
						{capture.identity.streamEpoch} · seq{" "}
						{capture.identity.streamSequence}
					</span>
				}
			/>
			<Field
				label="capture integrity"
				value={
					<span
						className={
							run?.integrity === "COMPLETE" ? "text-(--up)" : "text-(--warn)"
						}
					>
						{run?.integrity ?? "UNKNOWN"}
					</span>
				}
			/>
			<Field
				label="stream"
				value={<span className="text-[9px]">{capture.identity.stream}</span>}
			/>
			<Field
				label="endpoint"
				value={<span className="text-[9px]">{capture.endpoint || "—"}</span>}
			/>
			<Field label="received at" value={formatClock(capture.receivedAt)} />
			<Field
				label="run"
				value={
					<span className="text-[9px]">
						{capture.identity.run.slice(0, 18)}
					</span>
				}
			/>
		</div>
	);
};

/*
FrameStrip is the raw capture tape immediately around the playhead: the actual
external inputs, in the order SYMM observed them, each one clickable. It is the
irreducible substrate everything above is derived from.
*/
export const FrameStrip = ({
	captures,
	playhead,
	onSelect,
}: {
	captures: HindsightCapture[];
	playhead: number | null;
	onSelect: (sequence: number) => void;
}) => (
	<div className="flex gap-px overflow-x-auto border-(--line) border-b bg-(--sunken) px-1.5 py-1.5">
		{captures.length === 0 ? (
			<span className="px-1.5 font-mono text-[9px] text-(--f4)">
				no captured frames in this neighbourhood
			</span>
		) : null}
		{captures.map((capture) => {
			const active = playhead === capture.identity.sequence;

			return (
				<Button
					key={capture.identity.sequence}
					variant="bare"
					title={`capture ${capture.identity.sequence} · ${capture.kind} · ${capture.identity.stream} epoch ${capture.identity.streamEpoch}`}
					className={`shrink-0 rounded-[2px] border px-1 py-0.5 font-mono text-[8px] tabular-nums ${
						active
							? "border-(--f1) bg-(--raised) text-(--f1)"
							: "border-(--line) text-(--f4) hover:border-(--line2) hover:text-(--f2)"
					}`}
					onClick={() => onSelect(capture.identity.sequence)}
				>
					{capture.kind.slice(0, 4)}·{capture.identity.sequence}
				</Button>
			);
		})}
	</div>
);

/*
ProvenancePanel shows how the frame under the playhead entered Workspace and
what the running system produced from it: the envelopes it fanned out into, and
the artifacts witnessed at each boundary with the exact parents they consumed.
*/
export const ProvenancePanel = ({
	envelope,
	onSelect,
}: {
	envelope: HindsightEnvelope | null;
	onSelect: (sequence: number, ordinal: number) => void;
}) => {
	const boundaries = useMemo(() => {
		const grouped = new Map<string, HindsightEnvelope["witnesses"]>();

		for (const witness of envelope?.witnesses ?? []) {
			const existing = grouped.get(witness.boundary) ?? [];
			existing.push(witness);
			grouped.set(witness.boundary, existing);
		}

		return [...grouped.entries()];
	}, [envelope]);

	const rawPreview = useMemo(() => {
		if (typeof envelope?.payload !== "string" || envelope.payload === "") {
			return null;
		}

		try {
			return atob(envelope.payload).slice(0, 600);
		} catch {
			return null;
		}
	}, [envelope]);

	if (envelope === null || envelope.manifests === undefined) {
		return null;
	}

	return (
		<Flex.Column gap={3} className="p-3">
			<Section fit="content" surface="sunken">
				<Section.Header
					title="Workspace ingress"
					size="s"
					rule
					meta={
						<span className="font-mono text-[9px] text-(--f4)">
							{envelope.manifests.length} envelope
							{envelope.manifests.length === 1 ? "" : "s"} from this frame
						</span>
					}
				/>
				<Section.Body>
					{envelope.manifests.length === 0 ? (
						<p className="px-2.5 py-2 font-mono text-[9px] text-(--f4)">
							This frame produced no semantic envelope. The raw frame still
							exists — a heartbeat or a protocol acknowledgement is a captured
							input with zero envelopes, not a missing record.
						</p>
					) : null}
					<ul className="flex flex-col divide-y divide-(--line)">
						{envelope.manifests.map((manifest) => (
							<li
								key={manifest.envelope.ordinal}
								className="flex items-center justify-between px-2.5 py-1 font-mono text-[9px]"
							>
								<span className="text-(--f1)">
									<span className="text-(--acc)">
										{envelope.sequence}:{manifest.envelope.ordinal}
									</span>{" "}
									{manifest.symbol || "—"}
								</span>
								<span className="text-(--f4)">
									{manifest.workload} · {manifest.domainKind}
								</span>
							</li>
						))}
					</ul>
				</Section.Body>
			</Section>

			<Section fit="content" surface="sunken">
				<Section.Header
					title="Artifact witnesses"
					size="s"
					rule
					meta={
						<span className="font-mono text-[9px] text-(--f4)">
							{envelope.witnesses.length} recorded
						</span>
					}
				/>
				<Section.Body>
					{boundaries.length === 0 ? (
						<p className="px-2.5 py-2 font-mono text-[9px] text-(--f4)">
							No artifact was witnessed at any boundary for this frame.
						</p>
					) : null}
					{boundaries.map(([boundary, witnesses]) => (
						<div
							key={boundary}
							className="border-(--line) border-b last:border-b-0"
						>
							<div className="bg-(--surface) px-2.5 py-1 font-mono text-[8px] text-(--acc) uppercase tracking-widest">
								{boundary}
							</div>
							<ul className="flex flex-col">
								{witnesses.map((witness) => {
									const parents = witness.immediateParents ?? [];

									return (
										<li
											key={`${witness.artifact.kind}-${witness.artifact.identity}-${witness.envelope.ordinal}`}
											className="px-2.5 py-1 font-mono text-[9px] text-(--f1)"
										>
											<Flex.Row align="center" justify="between" gap={2}>
												<span className="truncate">
													{witness.artifact.kind}
													<span className="text-(--f3)">
														:{witness.artifact.identity}
													</span>
												</span>
												{witness.component ? (
													<span className="shrink-0 text-(--f4)">
														{witness.component}
														<span className="text-(--f3)">
															@v{witness.componentStateVersion ?? 0}
														</span>
													</span>
												) : null}
											</Flex.Row>
											{parents.length > 0 ? (
												<Flex.Row gap={1} className="mt-0.5 flex-wrap">
													<span className="text-[8px] text-(--f4)">
														parents
													</span>
													{parents.map((parent) => (
														<Button
															key={`${parent.origin.sequence}-${parent.ordinal}`}
															variant="bare"
															className="rounded-[2px] border border-(--line2) px-1 font-mono text-[8px] text-(--f3) tabular-nums hover:border-(--acc) hover:text-(--f1)"
															onClick={() =>
																onSelect(parent.origin.sequence, parent.ordinal)
															}
														>
															{parent.origin.sequence}:{parent.ordinal}
														</Button>
													))}
												</Flex.Row>
											) : null}
										</li>
									);
								})}
							</ul>
						</div>
					))}
				</Section.Body>
			</Section>

			{rawPreview !== null ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Raw frame" size="s" rule />
					<Section.Body>
						<pre className="overflow-x-auto whitespace-pre-wrap break-all px-2.5 py-2 font-mono text-[9px] text-(--f3) leading-relaxed">
							{rawPreview}
						</pre>
					</Section.Body>
				</Section>
			) : null}
		</Flex.Column>
	);
};

/*
A decoded signal Measurement, read once out of the flatbuffer so the panel can
sort, key, and render it without reaching back into the buffer on every frame.

Presence is carried explicitly at every level. A metric that was never
normalised is undefined, not zero, and an estimator that could not estimate its
dispersion has no SNR rather than an SNR of nought — collapsing either into a
convenient number would erase exactly the distinction this view exists to show.
*/
type MetricReading = {
	id: string;
	key: string;
	label: string;
	raw: number;
	normalized: number | null;
	standardized: number | null;
	unit: string;
	timescale: string;
};

type MeasurementReading = {
	signal: string;
	id: string;
	label: string;
	source: string;
	seqIdx: string;
	at: number;
	from: number | null;
	maturity: number;
	snr: number | null;
	metrics: MetricReading[];
	metadata: Array<{ id: string; name: string; value: number }>;
	provenance: Array<{ id: string; name: string; value: string }>;
};

const readMeasurement = (
	signal: string,
	measurement: EnvelopeMeasurement,
): MeasurementReading => {
	const metrics: MetricReading[] = [];

	for (let position = 0; position < measurement.metricsLength(); position++) {
		const entry = measurement.metrics(position);
		const metric = entry?.value();

		if (entry === null || metric === null || metric === undefined) continue;

		const key = entry.key() ?? "";

		metrics.push({
			id: `${position}:${key}`,
			key,
			label: metric.label() ?? "",
			raw: metric.raw(),
			normalized: metric.hasNormalized() ? metric.normalized() : null,
			standardized: metric.hasStandardized() ? metric.standardized() : null,
			unit: metric.unit() ?? "",
			timescale: metric.timescale() ?? "",
		});
	}

	const metadata: MeasurementReading["metadata"] = [];

	for (let position = 0; position < measurement.metadataLength(); position++) {
		const entry = measurement.metadata(position);

		if (entry === null) continue;

		metadata.push({
			id: `${position}:${entry.name()}`,
			name: entry.name() ?? "",
			value: entry.value(),
		});
	}

	const provenance: MeasurementReading["provenance"] = [];

	for (
		let position = 0;
		position < measurement.provenanceLength();
		position++
	) {
		const entry = measurement.provenance(position);

		if (entry === null) continue;

		provenance.push({
			id: `${position}:${entry.name()}`,
			name: entry.name() ?? "",
			value: entry.value() ?? "",
		});
	}

	return {
		signal,
		id: measurement.id() ?? signal,
		label: measurement.label() ?? "",
		source: measurement.source() ?? "",
		seqIdx: measurement.seqIdx().toString(),
		at: Number(measurement.atNs()),
		from: measurement.hasFrom() ? Number(measurement.fromNs()) : null,
		maturity: measurement.maturity(),
		snr: measurement.snrDefined() ? measurement.snr() : null,
		metrics,
		metadata,
		provenance,
	};
};

/*
formatValue keeps a metric legible across the range these estimators actually
produce — arrival intensities in the thousands beside imbalances at 1e-9 —
without rounding a small one into a flat zero it never was.
*/
const formatValue = (value: number): string => {
	if (!Number.isFinite(value)) return String(value);
	if (value === 0) return "0";

	const magnitude = Math.abs(value);

	if (magnitude >= 1e6 || magnitude < 1e-4) return value.toExponential(3);
	if (magnitude >= 100) return value.toFixed(2);
	if (magnitude >= 1) return value.toFixed(4);

	return value.toFixed(6);
};

const formatNanos = (nanos: number): string => {
	if (!Number.isFinite(nanos) || nanos <= 0) return "—";

	return new Date(nanos / 1e6).toLocaleTimeString([], {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		hour12: false,
	});
};

/*
Undefined is rendered as the word, in the warning tone, wherever a quantity was
not estimable. It must never look like a value.
*/
const Quantity = ({ value }: { value: number | null }) =>
	value === null ? (
		<span
			className="text-(--warn)"
			title="Not estimable here. Undefined, not zero."
		>
			undef
		</span>
	) : (
		<span className="text-(--f1) tabular-nums">{formatValue(value)}</span>
	);

/*
MetricDetail states what one number physically means, quoting METRIC_MAP.md
rather than summarising it.

The forbidden-use line is not decoration. It is the declared boundary on what
may be concluded from this metric, and an inspection surface that hid it would
be inviting exactly the inference the map exists to forbid. A metric with no
entry says so plainly: an undeclared metric has no meaning this UI is entitled
to supply.
*/
const MetricDetail = ({
	identity,
	declared,
	referenced,
	observedAt,
	version,
}: {
	identity: string;
	declared: MetricSemantics | null;
	referenced: Array<{ category: string; stance: string }>;
	observedAt: string;
	version: { component: string; version: number } | null;
}) => (
	<Flex.Column gap={1} className="font-mono text-[8px] leading-relaxed">
		<Flex.Row gap={3} className="flex-wrap text-(--f4)">
			<span>
				identity <span className="text-(--f2)">{identity}</span>
			</span>
			<span>
				observed at <span className="text-(--f2)">{observedAt}</span>
			</span>
			{version === null ? null : (
				<span>
					state version{" "}
					<span className="text-(--f2)">
						{version.component}@v{version.version}
					</span>
				</span>
			)}
			{declared?.role ? (
				<span>
					role <span className="text-(--acc)">{declared.role}</span>
				</span>
			) : null}
			{declared?.class ? (
				<span>
					class <span className="text-(--f2)">{declared.class}</span>
				</span>
			) : null}
		</Flex.Row>

		{declared === null ? (
			<p className="text-(--warn)">
				Not declared in METRIC_MAP.md. This surface has no statement of what the
				number means, and will not invent one.
			</p>
		) : (
			<>
				{declared.purpose ? (
					<p className="text-(--f2)">{declared.purpose}</p>
				) : null}
				{declared.destinations ? (
					<p className="text-(--f4)">
						<span className="uppercase tracking-widest">may inform</span>{" "}
						{declared.destinations}
					</p>
				) : null}
				{declared.forbidden ? (
					<p className="text-(--warn)">
						<span className="uppercase tracking-widest">never infer</span>{" "}
						{declared.forbidden}
					</p>
				) : null}
			</>
		)}

		{referenced.length > 0 ? (
			<Flex.Row gap={1} className="flex-wrap pt-0.5">
				<span
					className="text-(--f4) uppercase tracking-widest"
					title="Category hypotheses that named this exact metric as evidence at this boundary."
				>
					referenced by
				</span>
				{referenced.map((entry) => (
					<span
						key={`${entry.category}-${entry.stance}`}
						className="rounded-[2px] border border-(--line2) px-1"
						style={{
							color:
								entry.stance === "supports"
									? "var(--up)"
									: entry.stance === "contradicts"
										? "var(--down)"
										: "var(--f4)",
						}}
					>
						{entry.category} · {entry.stance}
					</span>
				))}
			</Flex.Row>
		) : null}
	</Flex.Column>
);

/*
MeasurementPanel is the Signals / Measurements view of a SystemSnapshot (§32):
for each signal the running binary produced here, its source and support, and
every metric it carried with the raw, normalised, and standardised value, the
unit, and the timescale it was measured over.

Rows open on click because a busy envelope carries hundreds of metrics; the
count alone is not evidence, so the values are always one click away rather
than a separate screen.
*/
const MeasurementPanel = ({
	measurements,
	semantics,
	versions,
	evidence,
}: {
	measurements: MeasurementReading[];
	semantics: HindsightMetricMap | null;
	versions: Map<string, { component: string; version: number }>;
	evidence: Map<string, Array<{ category: string; stance: string }>>;
}) => {
	const [open, setOpen] = useState<ReadonlySet<string>>(new Set());
	const [metric, setMetric] = useState<string | null>(null);

	const total = measurements.reduce(
		(sum, measurement) => sum + measurement.metrics.length,
		0,
	);
	const allOpen = open.size === measurements.length;

	return (
		<Section fit="content" surface="sunken">
			<Section.Header
				title="Signal measurements"
				size="s"
				rule
				meta={
					<Button
						variant="bare"
						className="font-mono text-[9px] text-(--f4) hover:text-(--f1)"
						onClick={() =>
							setOpen(
								allOpen
									? new Set()
									: new Set(
											measurements.map((measurement) => measurement.signal),
										),
							)
						}
					>
						{total} metrics · {allOpen ? "collapse all" : "expand all"}
					</Button>
				}
			/>
			<Section.Body>
				{measurements.map((measurement) => {
					const expanded = open.has(measurement.signal);

					return (
						<div
							key={measurement.signal}
							className="border-(--line) border-b last:border-b-0"
						>
							<Button
								variant="bare"
								className="flex w-full items-center gap-2 px-2.5 py-1 text-left font-mono text-[9px] hover:bg-(--raised)"
								onClick={() =>
									setOpen((current) => {
										const next = new Set(current);

										if (!next.delete(measurement.signal)) {
											next.add(measurement.signal);
										}

										return next;
									})
								}
							>
								<span className="w-2 text-(--f4)">{expanded ? "▾" : "▸"}</span>
								<span className="text-(--f1)">{measurement.signal}</span>
								<span className="ml-auto flex items-center gap-3 text-(--f4)">
									{versions.has(measurement.id) ? (
										<span title="The resident component version that participated, as its witness recorded it.">
											{versions.get(measurement.id)?.component}
											<span className="text-(--f2)">
												@v{versions.get(measurement.id)?.version}
											</span>
										</span>
									) : null}
									<span title="Estimator support, as the component reported it.">
										mat{" "}
										<span className="text-(--f2) tabular-nums">
											{measurement.maturity.toFixed(3)}
										</span>
									</span>
									<span title="Signal-to-noise, where the estimator could define one.">
										snr <Quantity value={measurement.snr} />
									</span>
									<span className="tabular-nums">
										{measurement.metrics.length} metrics
									</span>
								</span>
							</Button>

							{expanded ? (
								<div className="bg-(--bg) px-2.5 py-1.5">
									<div className="flex flex-wrap gap-x-4 gap-y-0.5 pb-1.5 font-mono text-[8px] text-(--f4)">
										<span>
											source{" "}
											<span className="text-(--f2)">
												{measurement.source || "—"}
											</span>
										</span>
										<span>
											at{" "}
											<span className="text-(--f2)">
												{formatNanos(measurement.at)}
											</span>
										</span>
										<span title="The start of the window this measurement covers, where the estimator declared one.">
											from{" "}
											<span className="text-(--f2)">
												{measurement.from === null
													? "—"
													: formatNanos(measurement.from)}
											</span>
										</span>
										<span>
											seq{" "}
											<span className="text-(--f2) tabular-nums">
												{measurement.seqIdx}
											</span>
										</span>
										{measurement.id ? (
											<span>
												id <span className="text-(--f2)">{measurement.id}</span>
											</span>
										) : null}
									</div>

									{measurement.metrics.length === 0 ? (
										<p className="py-1 font-mono text-[9px] text-(--f4)">
											This signal produced no metric at this boundary.
										</p>
									) : (
										<table className="w-full border-collapse font-mono text-[9px]">
											<thead>
												<tr className="text-(--f4) text-left">
													<th className="py-0.5 pr-2 font-normal">metric</th>
													<th className="py-0.5 pr-2 text-right font-normal">
														raw
													</th>
													<th className="py-0.5 pr-2 text-right font-normal">
														norm
													</th>
													<th className="py-0.5 pr-2 text-right font-normal">
														std
													</th>
													<th className="py-0.5 font-normal">
														unit · timescale
													</th>
												</tr>
											</thead>
											<tbody>
												{measurement.metrics.map((entry) => {
													const identity = `${measurement.signal}/${entry.key}`;
													const declared = semantics?.metrics[identity] ?? null;
													const referenced =
														evidence.get(
															`${measurement.signal}:${entry.key}`,
														) ?? [];
													const selected = metric === identity;

													return (
														<Fragment key={entry.id}>
															<tr
																className={`cursor-pointer border-(--line) border-t align-baseline hover:bg-(--raised) ${
																	selected ? "bg-(--raised)" : ""
																}`}
																onClick={() =>
																	setMetric((current) =>
																		current === identity ? null : identity,
																	)
																}
															>
																<td className="py-0.5 pr-2 text-(--f2)">
																	<span className="text-(--f4)">
																		{selected ? "▾" : "▸"}{" "}
																	</span>
																	{entry.key}
																	{entry.label && entry.label !== entry.key ? (
																		<span className="text-(--f4)">
																			{" "}
																			· {entry.label}
																		</span>
																	) : null}
																	{referenced.length > 0 ? (
																		<span
																			className="text-(--acc)"
																			title={`Referenced as evidence by ${referenced.length} category hypothesis/hypotheses at this boundary.`}
																		>
																			{" "}
																			↳{referenced.length}
																		</span>
																	) : null}
																</td>
																<td className="py-0.5 pr-2 text-right">
																	<Quantity value={entry.raw} />
																</td>
																<td className="py-0.5 pr-2 text-right">
																	<Quantity value={entry.normalized} />
																</td>
																<td className="py-0.5 pr-2 text-right">
																	<Quantity value={entry.standardized} />
																</td>
																<td className="py-0.5 text-(--f4)">
																	{entry.unit || "—"}
																	{entry.timescale
																		? ` · ${entry.timescale}`
																		: ""}
																</td>
															</tr>
															{selected ? (
																<tr className="border-(--line) border-t">
																	<td colSpan={5} className="px-2 py-1.5">
																		<MetricDetail
																			identity={identity}
																			declared={declared}
																			referenced={referenced}
																			observedAt={formatNanos(measurement.at)}
																			version={
																				versions.get(measurement.id) ?? null
																			}
																		/>
																	</td>
																</tr>
															) : null}
														</Fragment>
													);
												})}
											</tbody>
										</table>
									)}

									{measurement.metadata.length > 0 ? (
										<div className="flex flex-wrap gap-x-3 gap-y-0.5 pt-1.5 font-mono text-[8px] text-(--f4)">
											<span className="uppercase tracking-widest">
												metadata
											</span>
											{measurement.metadata.map((entry) => (
												<span key={entry.id}>
													{entry.name}{" "}
													<span className="text-(--f2) tabular-nums">
														{formatValue(entry.value)}
													</span>
												</span>
											))}
										</div>
									) : null}

									{measurement.provenance.length > 0 ? (
										<div className="flex flex-wrap gap-x-3 gap-y-0.5 pt-1 font-mono text-[8px] text-(--f4)">
											<span
												className="uppercase tracking-widest"
												title="The immediate causal inputs this measurement consumed."
											>
												provenance
											</span>
											{measurement.provenance.map((entry) => (
												<span key={entry.id}>
													{entry.name}{" "}
													<span className="text-(--f2)">{entry.value}</span>
												</span>
											))}
										</div>
									) : null}
								</div>
							) : null}
						</div>
					);
				})}
			</Section.Body>
		</Section>
	);
};

/*
StatePanel decodes the EnvelopeState the running binary persisted at this
frame's observe boundary. It is Historical Witness: what SYMM actually held,
not what today's build would now compute.
*/
export const StatePanel = ({
	state,
	resident,
	envelope,
	semantics,
}: {
	state: EnvelopeState | null;
	resident: HindsightResident | null;
	envelope: HindsightEnvelope | null;
	semantics: HindsightMetricMap | null;
}) => {
	const residentMeasurements = useMemo<MeasurementReading[]>(
		() =>
			(resident?.signals ?? []).map((measurement) => ({
				signal: measurement.source,
				id: measurement.identity ?? measurement.source,
				label: "",
				source: measurement.source,
				seqIdx: String(measurement.origin.origin.sequence),
				at: measurement.atNs,
				from: null,
				maturity: measurement.maturity,
				snr: measurement.snrDefined ? measurement.snr : null,
				metrics: measurement.metrics.map((metric, position) => ({
					id: `${position}:${metric.key}`,
					key: metric.key,
					label: metric.label ?? "",
					raw: metric.raw,
					normalized: metric.hasNormalized ? metric.normalized : null,
					standardized: metric.hasStandardized ? metric.standardized : null,
					unit: metric.unit ?? "",
					timescale: metric.timescale ?? "",
				})),
				metadata: [],
				provenance: [
					{
						id: "origin",
						name: "origin",
						value: `${measurement.origin.origin.sequence}:${measurement.origin.ordinal}`,
					},
					{
						id: "residency",
						name: "residency",
						value: measurement.carried
							? measurement.hasAge
								? `carried ${(measurement.ageNs / 1e6).toFixed(2)}ms`
								: "carried"
							: "fresh",
					},
				],
			})),
		[resident],
	);

	const residentEvidence = useMemo(() => {
		const byMetric = new Map<
			string,
			Array<{ category: string; stance: string }>
		>();

		for (const category of resident?.categories ?? []) {
			for (const identity of category.supporting ?? []) {
				const references = byMetric.get(identity) ?? [];
				references.push({ category: category.type, stance: "supports" });
				byMetric.set(identity, references);
			}

			for (const identity of category.opposing ?? []) {
				const references = byMetric.get(identity) ?? [];
				references.push({ category: category.type, stance: "contradicts" });
				byMetric.set(identity, references);
			}
		}

		return byMetric;
	}, [resident]);

	const rows = useMemo(() => {
		if (state === null) {
			return { categories: [], perspectives: [], boundaries: [] };
		}

		const categories = Array.from(
			{ length: state.categoriesLength() },
			(_, position) => {
				const category = state.categories(position);

				return category === null
					? null
					: {
							id: `${position}:${category.type()}`,
							type: category.type() ?? "—",
							confidence: category.confidence(),
						};
			},
		).filter((row) => row !== null);

		const perspectives = Array.from(
			{ length: state.perspectivesLength() },
			(_, position) => {
				const perspective = state.perspectives(position);

				return perspective === null
					? null
					: {
							id: `${position}:${perspective.symbol()}`,
							symbol: perspective.symbol() ?? "—",
							readings: perspective.readingsLength(),
						};
			},
		).filter((row) => row !== null);

		const boundaries = Array.from(
			{ length: state.boundariesLength() },
			(_, position) => {
				const stamp = state.boundaries(position);

				return stamp === null
					? null
					: {
							id: `${position}:${stamp.label()}`,
							label: stamp.label() ?? "—",
							seqCount: stamp.seqCount().toString(),
						};
			},
		).filter((row) => row !== null);

		return { categories, perspectives, boundaries };
	}, [state]);

	const measurements = useMemo(() => {
		if (state === null) return [];

		return (
			[
				{ signal: "cvd", value: state.cvd() },
				{ signal: "hawkes", value: state.hawkes() },
				{ signal: "depthFlow", value: state.depthFlow() },
				{ signal: "morphology", value: state.morphology() },
				{ signal: "liquidity", value: state.liquidity() },
				{ signal: "correlation", value: state.correlation() },
				{ signal: "leadLag", value: state.leadLag() },
				{ signal: "sentiment", value: state.sentiment() },
				{ signal: "pumpDump", value: state.pumpDump() },
				{ signal: "toxicity", value: state.toxicity() },
				{ signal: "derivatives", value: state.derivatives() },
			] as Array<{ signal: string; value: EnvelopeMeasurement | null }>
		)
			.filter(
				(entry): entry is { signal: string; value: EnvelopeMeasurement } =>
					entry.value !== null,
			)
			.map(({ signal, value }) => readMeasurement(signal, value));
	}, [state]);

	/*
		A measurement's component state version lives on its witness, not in the
		state payload — the witness is what recorded which resident version
		actually participated (§19). They are joined here by artifact identity,
		never by position or by timestamp.
	*/
	const versions = useMemo(() => {
		const byIdentity = new Map<
			string,
			{ component: string; version: number }
		>();

		for (const witness of envelope?.witnesses ?? []) {
			if (witness.artifact.kind !== "measurement") continue;

			byIdentity.set(witness.artifact.identity, {
				component: witness.component ?? "",
				version: witness.componentStateVersion ?? 0,
			});
		}

		return byIdentity;
	}, [envelope]);

	/*
		Category evidence names the metrics it consumed as "source:metric" — the
		same pair a metric row carries — so the forward edge from a value to the
		categories that referenced it is an exact identity join, not a guess.
	*/
	const evidence = useMemo(() => {
		const byMetric = new Map<
			string,
			Array<{ category: string; stance: string }>
		>();

		if (state === null) return byMetric;

		for (let position = 0; position < state.categoriesLength(); position++) {
			const category = state.categories(position);

			if (category === null) continue;

			const name = category.type() ?? "";
			const add = (identity: string | null, stance: string) => {
				if (identity === null || identity === "") return;

				const existing = byMetric.get(identity) ?? [];
				existing.push({ category: name, stance });
				byMetric.set(identity, existing);
			};

			for (let index = 0; index < category.supportingLength(); index++) {
				add(category.supporting(index), "supports");
			}

			for (let index = 0; index < category.opposingLength(); index++) {
				add(category.opposing(index), "contradicts");
			}

			for (let index = 0; index < category.missingLength(); index++) {
				add(category.missing(index), "missing");
			}
		}

		return byMetric;
	}, [state]);

	if (state === null) {
		if (resident !== null && residentMeasurements.length > 0) {
			return (
				<Flex.Column gap={3} className="p-3">
					<div className="font-mono text-[9px] text-(--f4) leading-relaxed">
						<span className="text-(--acc)">
							Resident state as-of this envelope
						</span>
						{" · "}latest causally available values, with their exact origins
						and ages. Examined {resident.examined} envelopes and reached back{" "}
						{resident.reachedBack} captures.
					</div>

					<MeasurementPanel
						measurements={residentMeasurements}
						semantics={semantics}
						versions={new Map()}
						evidence={residentEvidence}
					/>

					{resident.categories.length > 0 ? (
						<Section fit="content" surface="sunken">
							<Section.Header title="Resident categories" size="s" rule />
							<Section.Body>
								{resident.categories.map((category) => (
									<div
										key={`${category.type}:${category.origin.origin.sequence}:${category.origin.ordinal}`}
										className="flex items-center justify-between px-2.5 py-1 font-mono text-[9px]"
									>
										<span className="text-(--f1)">{category.type}</span>
										<span className="text-(--f4) tabular-nums">
											conf {category.confidence.toFixed(3)} · origin{" "}
											{category.origin.origin.sequence}:
											{category.origin.ordinal}
										</span>
									</div>
								))}
							</Section.Body>
						</Section>
					) : null}

					{(resident.unresolved?.length ?? 0) > 0 ? (
						<p className="font-mono text-[9px] text-(--warn)">
							Unresolved within this causal walk:{" "}
							{resident.unresolved?.join(", ")}.
						</p>
					) : null}
				</Flex.Column>
			);
		}

		return (
			<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
				No exact or resident historical state was found at this envelope.
			</p>
		);
	}

	const strategy = state.strategy();

	return (
		<Flex.Column gap={3} className="p-3">
			<Flex.Row gap={4} className="flex-wrap font-mono text-[9px] text-(--f4)">
				<span>
					seq{" "}
					<span className="text-(--f1) tabular-nums">
						{state.captureSeq().toString()}
					</span>
				</span>
				<span>
					ordinal{" "}
					<span className="text-(--f1) tabular-nums">
						{state.captureOrdinal().toString()}
					</span>
				</span>
				<span>
					type <span className="text-(--f1)">{state.typeId()}</span>
				</span>
				<span>
					tick{" "}
					<span className="text-(--f1) tabular-nums">
						{state.tick().toString()}
					</span>
				</span>
				<span>
					key <span className="text-(--f1)">{state.key() ?? "—"}</span>
				</span>
			</Flex.Row>

			{measurements.length > 0 ? (
				<MeasurementPanel
					measurements={measurements}
					semantics={semantics}
					versions={versions}
					evidence={evidence}
				/>
			) : null}

			{rows.categories.length > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Categories" size="s" rule />
					<Section.Body>
						{rows.categories.map((category) => (
							<div
								key={category.id}
								className="flex items-center justify-between px-2.5 py-1 font-mono text-[9px]"
							>
								<span className="text-(--f1)">{category.type}</span>
								<span className="tabular-nums text-(--f4)">
									conf {category.confidence.toFixed(3)}
								</span>
							</div>
						))}
					</Section.Body>
				</Section>
			) : null}

			{rows.perspectives.length > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Legacy advisor readings" size="s" rule />
					<Section.Body>
						{rows.perspectives.map((perspective) => (
							<div
								key={perspective.id}
								className="flex items-center justify-between px-2.5 py-1 font-mono text-[9px]"
							>
								<span className="text-(--f1)">{perspective.symbol}</span>
								<span className="tabular-nums text-(--f4)">
									{perspective.readings} readings
								</span>
							</div>
						))}
					</Section.Body>
				</Section>
			) : null}

			{strategy !== null ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Strategy" size="s" rule />
					<Section.Body>
						<div className="px-2.5 py-1 font-mono text-[9px] text-(--f1)">
							{strategy.outcome() ?? "—"} · {strategy.decisionsLength()}{" "}
							decisions
						</div>
					</Section.Body>
				</Section>
			) : null}

			{rows.boundaries.length > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Boundary trace" size="s" rule />
					<Section.Body>
						{rows.boundaries.map((stamp) => (
							<div
								key={stamp.id}
								className="flex items-center justify-between px-2.5 py-1 font-mono text-[9px]"
							>
								<span className="text-(--f1)">{stamp.label}</span>
								<span className="tabular-nums text-(--f4)">
									{stamp.seqCount} seq
								</span>
							</div>
						))}
					</Section.Body>
				</Section>
			) : null}
		</Flex.Column>
	);
};
