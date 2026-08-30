import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import * as flatbuffers from "flatbuffers";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import {
	fetchHindsightCaptures,
	fetchHindsightEnvelope,
	fetchHindsightGaps,
	fetchHindsightLifecycle,
	fetchHindsightRuns,
	fetchHindsightState,
} from "#/components/hindsight/hindsight-api";
import type {
	HindsightCapture,
	HindsightEnvelope,
	HindsightGap,
	HindsightLifecycleEvent,
	HindsightRun,
} from "#/components/hindsight/hindsight-types";

const decodeEnvelopeState = (payload: unknown): EnvelopeState | null => {
	if (typeof payload !== "string") {
		return null;
	}

	const bytes = Uint8Array.from(atob(payload), (char) => char.charCodeAt(0));

	return EnvelopeState.getRootAsEnvelopeState(new flatbuffers.ByteBuffer(bytes));
};

const formatDigest = (digest?: string | null): string =>
	digest == null || digest === "" ? "—" : digest.slice(0, 12);

const HindsightRoute = () => {
	const [runs, setRuns] = useState<HindsightRun[]>([]);
	const [selectedRun, setSelectedRun] = useState<string | null>(null);
	const [captures, setCaptures] = useState<HindsightCapture[]>([]);
	const [selectedSeq, setSelectedSeq] = useState<number | null>(null);
	const [selectedOrdinal, setSelectedOrdinal] = useState<number>(0);
	const [state, setState] = useState<EnvelopeState | null>(null);
	const [envelope, setEnvelope] = useState<HindsightEnvelope | null>(null);
	const [gaps, setGaps] = useState<HindsightGap[]>([]);
	const [lifecycle, setLifecycle] = useState<HindsightLifecycleEvent[]>([]);

	useEffect(() => {
		let cancelled = false;

		fetchHindsightRuns().then((loaded) => {
			if (!cancelled) setRuns(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, []);

	useEffect(() => {
		if (selectedRun === null) return;

		let cancelled = false;

		fetchHindsightCaptures(selectedRun, 0).then((loaded) => {
			if (cancelled) return;
			setCaptures(loaded);
			setSelectedSeq(null);
			setSelectedOrdinal(0);
			setState(null);
			setEnvelope(null);
		});

		fetchHindsightGaps(selectedRun).then((loaded) => {
			if (!cancelled) setGaps(loaded);
		});

		fetchHindsightLifecycle(selectedRun).then((loaded) => {
			if (!cancelled) setLifecycle(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, [selectedRun]);

	useEffect(() => {
		if (selectedRun === null || selectedSeq === null) return;

		let cancelled = false;

		fetchHindsightState(selectedRun, selectedSeq, selectedOrdinal).then((loaded) => {
			if (cancelled) return;
			setState(loaded ? decodeEnvelopeState(loaded.payload) : null);
		});

		fetchHindsightEnvelope(selectedRun, selectedSeq).then((loaded) => {
			if (!cancelled) setEnvelope(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, [selectedRun, selectedSeq, selectedOrdinal]);

	const selectedRunMeta = useMemo(
		() => runs.find((run) => run.id === selectedRun) ?? null,
		[runs, selectedRun],
	);

	const ordinals = useMemo(
		() =>
			envelope === null
				? [0]
				: Array.from(
						new Set(envelope.manifests.map((manifest) => manifest.envelope.ordinal)),
					).sort((left, right) => left - right),
		[envelope],
	);

	return (
		<div className="flex h-full min-w-275 overflow-hidden bg-(--bg)">
			{/* Left — runs */}
			<Section fit="pane" surface="surface" className="w-64 shrink-0 border-r border-(--line)">
				<Section.Header title="Runs" size="lg" rule sticky />
				<Section.Body>
					{runs.length === 0 ? (
						<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
							No capture runs recorded yet.
						</p>
					) : null}
					<ul className="flex flex-col divide-y divide-(--line)">
						{runs.map((run) => {
							const active = selectedRun === run.id;

							return (
								<li key={run.id}>
									<Button
										variant="bare"
										className={`flex w-full flex-col items-start gap-1 px-3 py-2.5 text-left hover:bg-(--raised) ${active ? "bg-(--raised)" : ""}`}
										onClick={() => setSelectedRun(run.id)}
									>
										<Flex.Row align="center" justify="between" className="w-full">
											<span className="truncate font-mono text-[10px] font-semibold text-(--f1)">
												{formatDigest(run.id)}
											</span>
											<span
												className={`font-mono text-[8px] uppercase tracking-widest ${
													run.integrity === "COMPLETE" ? "text-(--up)" : "text-(--warn)"
												}`}
											>
												{run.integrity}
											</span>
										</Flex.Row>
										<span className="font-mono text-[9px] text-(--f4)">
											{run.startedAt ? new Date(run.startedAt).toLocaleString() : "—"}
										</span>
										<span className="truncate font-mono text-[8px] text-(--f4)">
											commit {formatDigest(run.codeCommit)} · build {formatDigest(run.buildId)}
										</span>
										<span className="font-mono text-[8px] text-(--f4)">
											cfg {formatDigest(run.configDigest)}
										</span>
									</Button>
								</li>
							);
						})}
					</ul>
				</Section.Body>
			</Section>

			{/* Middle — capture tape */}
			<Section fit="pane" surface="surface" className="w-72 shrink-0 border-r border-(--line)">
				<Section.Header title="Capture tape" size="lg" rule sticky />
				<Section.Body>
					{selectedRunMeta === null ? (
						<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
							Select a run to scrub its capture tape.
						</p>
					) : null}
					{captures.length === 0 && selectedRunMeta !== null ? (
						<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
							No captured frames in this run.
						</p>
					) : null}
					<ul className="flex flex-col divide-y divide-(--line)">
						{captures.map((capture) => {
							const active = selectedSeq === capture.identity.sequence;

							return (
								<li key={capture.identity.sequence}>
									<Button
										variant="bare"
										className={`flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left hover:bg-(--raised) ${active ? "bg-(--raised)" : ""}`}
										onClick={() => {
											setSelectedSeq(capture.identity.sequence);
											setSelectedOrdinal(0);
										}}
									>
										<Flex.Row align="center" justify="between" className="w-full">
											<span className="font-mono text-[10px] font-semibold tabular-nums text-(--f1)">
												#{capture.identity.sequence}
											</span>
											<span className="font-mono text-[9px] text-(--acc)">
												{capture.kind}
											</span>
										</Flex.Row>
										<span className="truncate font-mono text-[8px] text-(--f4)">
											{capture.identity.stream} · ep {capture.identity.streamEpoch} ·{" "}
											{capture.identity.streamSequence}
										</span>
									</Button>
								</li>
							);
						})}
					</ul>
				</Section.Body>
				{captures.length > 0 && selectedRun !== null ? (
					<div className="shrink-0 border-t border-(--line) px-3 py-2">
						<Button
							variant="outline"
							size="xs"
							className="w-full"
							onClick={() => {
								const last = captures[captures.length - 1];
								if (!last) return;

								fetchHindsightCaptures(
									selectedRun,
									last.identity.sequence,
								).then((loaded) => {
									setCaptures((previous) => [...previous, ...loaded]);
								});
							}}
						>
							Load more
						</Button>
					</div>
				) : null}
			</Section>

			{/* Right — inspection */}
			<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-hidden">
				<Flex.Row
					align="center"
					justify="between"
					className="h-11 shrink-0 border-b border-(--line) bg-(--surface) px-4"
				>
					<span className="font-mono text-[8px] uppercase tracking-widest text-(--up)">
						HISTORICAL · witness
					</span>
					<span className="font-mono text-[12px] font-semibold text-(--f1)">
						{selectedSeq === null ? "No capture selected" : `capture #${selectedSeq}`}
					</span>
					{selectedSeq !== null && ordinals.length > 0 ? (
						<Flex.Row align="center" gap={1}>
							{ordinals.map((ordinal) => (
								<Button
									key={ordinal}
									variant={selectedOrdinal === ordinal ? "solid" : "bare"}
									size="xs"
									className="h-5 px-2 font-mono text-[9px]"
									onClick={() => setSelectedOrdinal(ordinal)}
								>
									ord {ordinal}
								</Button>
							))}
						</Flex.Row>
					) : null}
				</Flex.Row>

				<div className="min-h-0 flex-1 overflow-auto">
					{gaps.length > 0 ? <GapsPanel gaps={gaps} /> : null}
					{lifecycle.length > 0 ? <LifecyclePanel events={lifecycle} /> : null}
					{state === null && envelope !== null ? (
						<EnvelopeDetail envelope={envelope} />
					) : null}
					{state !== null ? <StateDetail state={state} /> : null}
					{state === null && envelope === null ? (
						<p className="px-4 py-8 font-mono text-[10px] text-(--f4)">
							Scrub to a capture to inspect its exact recorded state and provenance.
						</p>
					) : null}
				</div>
			</Flex.Column>
		</div>
	);
};

const GapsPanel = ({ gaps }: { gaps: HindsightGap[] }) => (
	<Flex.Column gap={1} className="border-b border-(--line) bg-(--sunken) px-4 py-2.5">
		<span className="font-mono text-[8px] uppercase tracking-widest text-(--warn)">
			Integrity defects ({gaps.length})
		</span>
		{gaps.map((gap, index) => (
			<div key={`${gap.encoding}-${index}`} className="font-mono text-[9px] text-(--f3)">
				<span className="text-(--warn)">{gap.encoding}</span>
				{gap.sequence > 0 ? <span className="text-(--f4)"> @ seq {gap.sequence}</span> : null}
				{" · "}
				<span className="text-(--f4)">{gap.detail}</span>
			</div>
		))}
	</Flex.Column>
);

const LifecyclePanel = ({ events }: { events: HindsightLifecycleEvent[] }) => (
	<Flex.Column gap={1} className="border-b border-(--line) bg-(--sunken) px-4 py-2.5">
		<span className="font-mono text-[8px] uppercase tracking-widest text-(--acc)">
			Trading lifecycle ({events.length})
		</span>
		{events.map((event, index) => (
			<div key={`${event.decisionId}-${index}`} className="font-mono text-[9px] text-(--f3)">
				<span className="text-(--acc)">{event.kind}</span>
				{" · "}
				<span className="text-(--f1)">{event.symbol}</span>
				{event.action ? <span className="text-(--f4)"> ({event.action})</span> : null}
				{" · "}
				<span className="text-(--f4)">decision {event.decisionId}</span>
			</div>
		))}
	</Flex.Column>
);

const EnvelopeDetail = ({ envelope }: { envelope: HindsightEnvelope }) => {
	const rawPreview = useMemo(() => {
		if (typeof envelope.payload !== "string" || envelope.payload === "") return null;

		try {
			return atob(envelope.payload).slice(0, 200);
		} catch {
			return null;
		}
	}, [envelope.payload]);

	return (
		<Flex.Column gap={3} className="p-4">
			<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
				Envelope #{envelope.sequence}
			</span>
			<Flex.Column gap={1}>
				<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					manifests
				</span>
				{envelope.manifests.length === 0 ? (
					<p className="font-mono text-[10px] text-(--f4)">none</p>
				) : (
					<ul className="flex flex-col divide-y divide-(--line)">
						{envelope.manifests.map((manifest, index) => (
							<li key={index} className="py-1 font-mono text-[10px] text-(--f1)">
								ord {manifest.envelope.ordinal} · {manifest.workload} · {manifest.symbol}
							</li>
						))}
					</ul>
				)}
			</Flex.Column>
			<Flex.Column gap={1}>
				<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					artifact witnesses
				</span>
				{envelope.witnesses.length === 0 ? (
					<p className="font-mono text-[10px] text-(--f4)">none</p>
				) : (
					<ul className="flex flex-col divide-y divide-(--line)">
						{envelope.witnesses.map((witness, index) => (
							<li key={index} className="py-1 font-mono text-[10px] text-(--f1)">
								{witness.boundary} · {witness.artifact.kind}:{witness.artifact.identity}
								{witness.component ? (
									<span className="text-(--f4)">
										{" · "}
										{witness.component}@v{witness.componentStateVersion}
									</span>
								) : null}
								{witness.immediateParents.length > 0 ? (
									<span className="text-(--f4)">
										{" · parents "}
										{witness.immediateParents
											.map((parent) => `${parent.origin.sequence}:${parent.ordinal}`)
											.join(", ")}
									</span>
								) : null}
							</li>
						))}
					</ul>
				)}
			</Flex.Column>
			{rawPreview !== null ? (
				<Flex.Column gap={1}>
					<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						raw bytes (prefix)
					</span>
					<pre className="overflow-x-auto font-mono text-[9px] leading-relaxed text-(--f3)">
						{rawPreview}
					</pre>
				</Flex.Column>
			) : null}
		</Flex.Column>
	);
};

const StateDetail = ({ state }: { state: EnvelopeState }) => {
	const categoryCount = state.categoriesLength();
	const perspectiveCount = state.perspectivesLength();
	const boundaryCount = state.boundariesLength();

	const measurementFields = useMemo(
		() =>
			[
				{ label: "cvd", value: state.cvd() },
				{ label: "hawkes", value: state.hawkes() },
				{ label: "depthFlow", value: state.depthFlow() },
				{ label: "morphology", value: state.morphology() },
				{ label: "liquidity", value: state.liquidity() },
				{ label: "correlation", value: state.correlation() },
				{ label: "leadLag", value: state.leadLag() },
				{ label: "sentiment", value: state.sentiment() },
				{ label: "pumpDump", value: state.pumpDump() },
				{ label: "toxicity", value: state.toxicity() },
				{ label: "derivatives", value: state.derivatives() },
			].filter(
				(
					entry,
				): entry is { label: string; value: EnvelopeMeasurement } =>
					entry.value !== null,
			),
		[state],
	);

	return (
		<Flex.Column gap={4} className="p-4">
			<Flex.Row gap={4} className="font-mono text-[9px] text-(--f4)">
				<span>
					run <span className="text-(--f1)">{formatDigest(state.captureRun())}</span>
				</span>
				<span>
					seq <span className="text-(--f1)">{state.captureSeq().toString()}</span>
				</span>
				<span>
					type <span className="text-(--f1)">{state.typeId()}</span>
				</span>
				<span>
					tick <span className="text-(--f1)">{state.tick().toString()}</span>
				</span>
			</Flex.Row>

			{categoryCount > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Categories" size="s" rule />
					<Section.Body>
						{Array.from({ length: categoryCount }, (_, index) => {
							const category = state.categories(index);
							if (!category) return null;
							return (
								<div
									key={`${category.type()}-${index}`}
									className="flex items-center justify-between py-1 font-mono text-[10px]"
								>
									<span className="text-(--f1)">{category.type()}</span>
									<span className="tabular-nums text-(--f4)">
										conf {category.confidence().toFixed(2)}
									</span>
								</div>
							);
						})}
					</Section.Body>
				</Section>
			) : null}

			{measurementFields.length > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Signal measurements" size="s" rule />
					<Section.Body>
						{measurementFields.map(({ label, value }) => (
							<div
								key={label}
								className="flex items-center justify-between py-1 font-mono text-[10px]"
							>
								<span className="text-(--f1)">{label}</span>
								<span className="tabular-nums text-(--f4)">
									{value.metricsLength()} metrics
								</span>
							</div>
						))}
					</Section.Body>
				</Section>
			) : null}

			{perspectiveCount > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Perspectives" size="s" rule />
					<Section.Body>
						{Array.from({ length: perspectiveCount }, (_, index) => {
							const perspective = state.perspectives(index);
							if (!perspective) return null;
							return (
								<div
									key={`${perspective.symbol()}-${index}`}
									className="flex items-center justify-between py-1 font-mono text-[10px]"
								>
									<span className="text-(--f1)">{perspective.symbol()}</span>
									<span className="tabular-nums text-(--f4)">
										{perspective.readingsLength()} readings
									</span>
								</div>
							);
						})}
					</Section.Body>
				</Section>
			) : null}

			{state.strategy() !== null ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Strategy / decision" size="s" rule />
					<Section.Body>
						<div className="py-1 font-mono text-[10px] text-(--f1)">
							{state.strategy()?.outcome() ?? "—"} · decisions{" "}
							{state.strategy()?.decisionsLength() ?? 0}
						</div>
					</Section.Body>
				</Section>
			) : null}

			{state.equity() !== null ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Account equity" size="s" rule />
					<Section.Body>
						<div className="py-1 font-mono text-[10px] text-(--f1)">
							equity {state.equity()?.equity() ?? "—"}
						</div>
					</Section.Body>
				</Section>
			) : null}

			{boundaryCount > 0 ? (
				<Section fit="content" surface="sunken">
					<Section.Header title="Boundary trace" size="s" rule />
					<Section.Body>
						{Array.from({ length: boundaryCount }, (_, index) => {
							const stamp = state.boundaries(index);
							if (!stamp) return null;
							return (
								<div
									key={`${stamp.label()}-${index}`}
									className="flex items-center justify-between py-1 font-mono text-[10px]"
								>
									<span className="text-(--f1)">{stamp.label()}</span>
									<span className="tabular-nums text-(--f4)">
										{stamp.seqCount().toString()}
									</span>
								</div>
							);
						})}
					</Section.Body>
				</Section>
			) : null}

			{categoryCount === 0 &&
			measurementFields.length === 0 &&
			boundaryCount === 0 ? (
				<p className="font-mono text-[10px] text-(--f4)">
					This envelope produced no semantic artifacts at its Observe boundary.
				</p>
			) : null}
		</Flex.Column>
	);
};

export const Route = createFileRoute("/hindsight")({
	component: HindsightRoute,
});
