import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import * as flatbuffers from "flatbuffers";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import {
	fetchHindsightCaptures,
	fetchHindsightRuns,
	fetchHindsightStates,
} from "#/components/hindsight/hindsight-api";
import type {
	HindsightCapture,
	HindsightRun,
} from "#/components/hindsight/hindsight-types";

/*
decodeEnvelopeState reconstructs one persisted EnvelopeState flatbuffer from the
base64 JSON payload the /hindsight/states endpoint returns. The payload is the
exact bytes the witness node persisted — the same EnvelopeState class the live
stream decodes — so a scrubbed historical point reads as the running binary
actually produced it.
*/
const decodeEnvelopeState = (payload: unknown): EnvelopeState | null => {
	if (typeof payload !== "string") {
		return null;
	}

	const bytes = Uint8Array.from(atob(payload), (char) => char.charCodeAt(0));
	const buffer = new flatbuffers.ByteBuffer(bytes);

	return EnvelopeState.getRootAsEnvelopeState(buffer);
};

const formatDigest = (digest?: string | null): string =>
	digest == null || digest === "" ? "—" : digest.slice(0, 12);

const HindsightRoute = () => {
	const [runs, setRuns] = useState<HindsightRun[]>([]);
	const [selectedRun, setSelectedRun] = useState<string | null>(null);
	const [captures, setCaptures] = useState<HindsightCapture[]>([]);
	const [selectedSeq, setSelectedSeq] = useState<number | null>(null);
	const [state, setState] = useState<EnvelopeState | null>(null);

	useEffect(() => {
		let cancelled = false;

		fetchHindsightRuns().then((loaded) => {
			if (!cancelled) {
				setRuns(loaded);
			}
		});

		return () => {
			cancelled = true;
		};
	}, []);

	useEffect(() => {
		if (selectedRun === null) {
			return;
		}

		let cancelled = false;

		fetchHindsightCaptures(selectedRun).then((loaded) => {
			if (!cancelled) {
				setCaptures(loaded);
				setSelectedSeq(null);
				setState(null);
			}
		});

		return () => {
			cancelled = true;
		};
	}, [selectedRun]);

	useEffect(() => {
		if (selectedRun === null || selectedSeq === null) {
			return;
		}

		let cancelled = false;

		fetchHindsightStates(selectedRun).then((loaded) => {
			if (cancelled) {
				return;
			}

			const match = loaded.find(
				(entry) =>
					entry.envelope.origin.sequence === selectedSeq &&
					entry.envelope.ordinal === 0,
			);

			setState(match ? decodeEnvelopeState(match.payload) : null);
		});

		return () => {
			cancelled = true;
		};
	}, [selectedRun, selectedSeq]);

	const selectedRunMeta = useMemo(
		() => runs.find((run) => run.id === selectedRun) ?? null,
		[runs, selectedRun],
	);

	return (
		<div className="flex h-full min-w-275 overflow-hidden bg-(--bg)">
			{/* Left — runs */}
			<Section
				fit="pane"
				surface="surface"
				className="w-64 shrink-0 border-r border-(--line)"
			>
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
													run.integrity === "COMPLETE"
														? "text-(--up)"
														: "text-(--warn)"
												}`}
											>
												{run.integrity}
											</span>
										</Flex.Row>
										<span className="font-mono text-[9px] text-(--f4)">
											{run.startedAt
												? new Date(run.startedAt).toLocaleString()
												: "—"}
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
			<Section
				fit="pane"
				surface="surface"
				className="w-72 shrink-0 border-r border-(--line)"
			>
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
										onClick={() => setSelectedSeq(capture.identity.sequence)}
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
											{capture.identity.stream} · ep {capture.identity.streamEpoch} · {capture.identity.streamSequence}
										</span>
									</Button>
								</li>
							);
						})}
					</ul>
				</Section.Body>
			</Section>

			{/* Right — exact system state */}
			<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-hidden">
				<Flex.Row
					align="center"
					justify="between"
					className="h-11 shrink-0 border-b border-(--line) bg-(--surface) px-4"
				>
					<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						System state
					</span>
					<span className="font-mono text-[12px] font-semibold text-(--f1)">
						{selectedSeq === null
							? "No capture selected"
							: `capture #${selectedSeq}`}
					</span>
				</Flex.Row>
				<div className="min-h-0 flex-1 overflow-auto p-4">
					{state === null ? (
						<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
							Scrub to a capture to see its exact recorded state.
						</p>
					) : null}
					{state !== null ? <StateDetail state={state} /> : null}
				</div>
			</Flex.Column>
		</div>
	);
};

const StateDetail = ({ state }: { state: EnvelopeState }) => {
	const categoryCount = state.categoriesLength();
	const measurementCount = state.boundariesLength();

	return (
		<Flex.Column gap={4}>
			<Flex.Row gap={4} className="font-mono text-[9px] text-(--f4)">
				<span>
					run <span className="text-(--f1)">{state.captureRun() ?? "—"}</span>
				</span>
				<span>
					seq <span className="text-(--f1)">{state.captureSeq().toString()}</span>
				</span>
				<span>
					type{" "}
					<span className="text-(--f1)">{state.typeId()}</span>
				</span>
				<span>
					tick <span className="text-(--f1)">{state.tick().toString()}</span>
				</span>
			</Flex.Row>

			{categoryCount > 0 ? (
				<Flex.Column gap={1}>
					<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						categories
					</span>
					<ul className="flex flex-col divide-y divide-(--line)">
						{Array.from({ length: categoryCount }, (_, index) => {
							const category = state.categories(index);
							if (!category) return null;
							return (
								<li
									key={`${category.type()}-${index}`}
									className="flex items-center justify-between py-1 font-mono text-[10px]"
								>
									<span className="text-(--f1)">{category.type()}</span>
									<span className="tabular-nums text-(--f4)">
										conf {category.confidence().toFixed(2)}
									</span>
								</li>
							);
						})}
					</ul>
				</Flex.Column>
			) : null}

			{measurementCount > 0 ? (
				<Flex.Column gap={1}>
					<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						boundaries ({measurementCount})
					</span>
					<ul className="flex flex-col divide-y divide-(--line)">
						{Array.from({ length: measurementCount }, (_, index) => {
							const stamp = state.boundaries(index);
							if (!stamp) return null;
							return (
								<li
									key={`${stamp.label()}-${index}`}
									className="flex items-center justify-between py-1 font-mono text-[10px]"
								>
									<span className="text-(--f1)">{stamp.label()}</span>
									<span className="tabular-nums text-(--f4)">
										{stamp.seqCount().toString()}
									</span>
								</li>
							);
						})}
					</ul>
				</Flex.Column>
			) : null}

			{categoryCount === 0 && measurementCount === 0 ? (
				<p className="font-mono text-[10px] text-(--f4)">
					This capture produced no semantic artifacts at its Observe boundary.
				</p>
			) : null}
		</Flex.Column>
	);
};

export const Route = createFileRoute("/hindsight")({
	component: HindsightRoute,
});
