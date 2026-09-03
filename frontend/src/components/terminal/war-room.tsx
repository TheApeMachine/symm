import { useSelector } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect, useMemo, useRef, useState } from "react";
import { decisionStore } from "#/collections/app";
import {
	type DecisionTraceModel,
	readDecisionTrace,
} from "#/components/terminal/decision-trace-model";
import {
	drawWarRoomTree,
	layoutWarRoomTree,
	warRoomTreeFrom,
} from "#/components/terminal/warroom-draw";
import { Decision } from "#/providers/telemetry/telemetry/decision";

/*
The War Room: one symbol's complete decision reasoning, live.

It is deliberately a component and not a route. The same reasoning is wanted in
two places — the decision surface, and the modal opened from a held position —
and those are the same question asked about a different symbol, not two
features. Everything here takes a symbol and reads its own state.

The four regions answer the four questions in the order they are actually
asked: who spoke (the council), what the search explored (the tree), what the
causal model concluded per action (the branch ledger), and how much of the
answer was real rather than imagined (the provenance bar).
*/

const fmt = (value: number, digits = 4): string =>
	Number.isFinite(value) ? value.toFixed(digits) : "—";

const pct = (value: number): string =>
	Number.isFinite(value) ? `${(value * 100).toFixed(1)}%` : "—";

/*
readTrace pulls one symbol's decision out of the symbol-keyed store and packs
it back into a flatbuffer accessor's shape.

The store holds the unpacked DecisionT, which is the durable form; the trace
reader works against the accessor API. Packing here keeps a single reader for
both paths rather than a second parallel projection that could drift.
*/
const useTrace = (symbol: string): DecisionTraceModel | null => {
	const decision = useSelector(
		decisionStore,
		(state) => state.bySymbol[symbol],
	);

	return useMemo(() => {
		if (!decision) {
			return null;
		}

		const builder = new flatbuffers.Builder(1024);
		builder.finish(decision.pack(builder));

		const accessor = Decision.getRootAsDecision(
			new flatbuffers.ByteBuffer(builder.asUint8Array()),
		);

		return readDecisionTrace(accessor);
	}, [decision]);
};

/*
SearchCanvas paints the MCTS tree at the surface's real pixel size, redrawing
whenever the trace or the box changes.
*/
const SearchCanvas = ({ trace }: { trace: DecisionTraceModel }) => {
	const canvas = useRef<HTMLCanvasElement | null>(null);
	const host = useRef<HTMLDivElement | null>(null);
	const [box, setBox] = useState({ width: 0, height: 0 });

	useEffect(() => {
		const element = host.current;

		if (element === null) return;

		const observer = new ResizeObserver((entries) => {
			for (const entry of entries) {
				setBox({
					width: entry.contentRect.width,
					height: entry.contentRect.height,
				});
			}
		});

		observer.observe(element);

		return () => observer.disconnect();
	}, []);

	useEffect(() => {
		const element = canvas.current;

		if (element === null || box.width === 0 || box.height === 0) return;

		const context = element.getContext("2d");

		if (context === null) return;

		const ratio = globalThis.devicePixelRatio || 1;
		element.width = box.width * ratio;
		element.height = box.height * ratio;
		context.setTransform(ratio, 0, 0, ratio, 0, 0);

		const read = getComputedStyle(element);
		const styles = {
			line: read.getPropertyValue("--line2").trim() || "#333",
			accent: read.getPropertyValue("--acc").trim() || "#e0a030",
			text: read.getPropertyValue("--f2").trim() || "#ccc",
			muted: read.getPropertyValue("--f4").trim() || "#777",
			up: read.getPropertyValue("--up").trim() || "#4ac08a",
			down: read.getPropertyValue("--down").trim() || "#d4614f",
		};

		const tree = warRoomTreeFrom(trace.tree);
		const layout = layoutWarRoomTree(tree, box.width, box.height);

		drawWarRoomTree(
			context,
			tree,
			layout,
			box.width,
			box.height,
			styles,
		);
	}, [trace, box]);

	return (
		<div ref={host} className="relative min-h-0 flex-1">
			<canvas
				ref={canvas}
				className="absolute inset-0 h-full w-full"
				style={{ width: box.width, height: box.height }}
			/>
		</div>
	);
};

/*
CouncilStrip is the War Room proper: who deliberated, what they concluded, and
which reading vetoed or reinforced another.

A veto is not a low score. It is one advisor invalidating another's conclusion,
which is why it is drawn as its own statement rather than folded into the
confidence number.
*/
const CouncilStrip = ({ trace }: { trace: DecisionTraceModel }) => (
	<div className="flex flex-col gap-1.5 border-(--line) border-b px-3 py-2">
		<div className="flex items-baseline justify-between gap-3">
			<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
				war room · {trace.council.participants} advisor
				{trace.council.participants === 1 ? "" : "s"} deliberated
			</span>
			<span className="font-mono text-[10px]">
				<span className="text-(--f1)">
					{trace.council.dominantMove || "—"}
				</span>{" "}
				<span className="text-(--f4)">{pct(trace.council.confidence)}</span>
			</span>
		</div>

		{trace.council.synergies.length === 0 &&
		trace.council.vetoes.length === 0 ? (
			<span className="font-mono text-[9px] text-(--f4)">
				No advisor reinforced or invalidated another this round.
			</span>
		) : null}

		{trace.council.synergies.map((reason) => (
			<span key={reason} className="font-mono text-[9px] text-(--up)">
				+ {reason}
			</span>
		))}

		{trace.council.vetoes.map((reason) => (
			<span key={reason} className="font-mono text-[9px] text-(--down)">
				− {reason} <span className="text-(--f4)">(veto)</span>
			</span>
		))}
	</div>
);

/*
BranchLedger is the per-action causal ledger: for each action the search could
take, what it was worth, how much of that was rolled out versus imagined, and
what Pearl's interventional query concluded.

do(action) is printed only where the structural model identified it. An
unidentified query is shown as such, never as a zero — a failed identification
and a genuine zero expectation are different claims, and collapsing them is the
error this whole layer exists to prevent.
*/
const BranchLedger = ({ trace }: { trace: DecisionTraceModel }) => {
	const peak = trace.branches.reduce(
		(most, branch) => Math.max(most, branch.visits + branch.counterfactualMass),
		0,
	);

	return (
		<div className="border-(--line) border-t">
			<div className="flex items-baseline justify-between px-3 py-1.5">
				<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
					causal ledger · per action
				</span>
				<span className="font-mono text-[8px] text-(--f4)">
					real rollouts vs pearl counterfactuals
				</span>
			</div>

			<table className="w-full border-collapse font-mono text-[9px]">
				<thead>
					<tr className="text-(--f4)">
						<th className="px-3 py-1 text-left font-normal">action</th>
						<th className="py-1 text-left font-normal">evidence</th>
						<th className="py-1 pr-2 text-right font-normal">value</th>
						<th className="py-1 pr-2 text-right font-normal">mean</th>
						<th className="py-1 pr-3 text-right font-normal">do(a)</th>
					</tr>
				</thead>
				<tbody>
					{trace.branches.map((branch) => {
						const total = branch.visits + branch.counterfactualMass;
						const realShare = total === 0 ? 0 : branch.visits / total;
						const width = peak === 0 ? 0 : (total / peak) * 100;

						return (
							<tr
								key={branch.action}
								className={`border-(--line) border-t ${
									branch.pruned ? "opacity-45" : ""
								}`}
							>
								<td className="px-3 py-1">
									<span
										className={
											branch.action === trace.recommendedAction
												? "text-(--acc)"
												: "text-(--f2)"
										}
									>
										{branch.action}
									</span>
									{branch.pruned ? (
										<span
											className="text-(--down)"
											title="Causally rejected: an identified sibling decisively dominated this action, so selection withdrew it."
										>
											{" "}
											rejected
										</span>
									) : null}
								</td>

								<td className="py-1 pr-2">
									{/* The bar is the whole argument: filled = rolled out,
									    hollow = Pearl filled it in. */}
									<span
										className="flex h-1.5 w-24 overflow-hidden rounded-xs bg-(--sunken)"
										title={`${branch.visits} real rollouts · ${branch.counterfactualMass.toFixed(2)} counterfactual mass`}
									>
										<span
											className="h-full bg-(--f3)"
											style={{ width: `${width * realShare}%` }}
										/>
										<span
											className="h-full bg-(--acc) opacity-55"
											style={{ width: `${width * (1 - realShare)}%` }}
										/>
									</span>
								</td>

								<td className="py-1 pr-2 text-right text-(--f2) tabular-nums">
									{fmt(branch.blendedValue)}
								</td>
								<td className="py-1 pr-2 text-right text-(--f3) tabular-nums">
									{fmt(branch.meanReward)}
								</td>
								<td className="py-1 pr-3 text-right tabular-nums">
									{branch.causalExpectationDefined ? (
										<span className="text-(--f2)">
											{fmt(branch.causalExpectation)}
										</span>
									) : (
										<span
											className="text-(--warn)"
											title="The structural model could not identify this interventional query here. Not identified is not the same as zero."
										>
											unidentified
										</span>
									)}
								</td>
							</tr>
						);
					})}
				</tbody>
			</table>
		</div>
	);
};

/*
WarRoom is the reusable surface. It takes a symbol and nothing else, so it can
be mounted inside the decision board or inside a position modal without either
caller knowing how the reasoning is read.
*/
export const WarRoom = ({
	symbol,
	className,
}: {
	symbol: string;
	className?: string;
}) => {
	const trace = useTrace(symbol);

	if (trace === null) {
		return (
			<div
				className={`flex min-h-0 items-center justify-center px-4 py-8 ${className ?? ""}`}
			>
				<span className="font-mono text-[10px] text-(--f4) leading-relaxed">
					No search ran for {symbol} this round — the council was silent, or no
					transition model was available.
				</span>
			</div>
		);
	}

	return (
		<div className={`flex min-h-0 flex-col ${className ?? ""}`}>
			<CouncilStrip trace={trace} />

			<div className="flex items-baseline justify-between px-3 py-1.5">
				<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
					causal search · {trace.iterations} rollouts × {trace.horizon} steps
				</span>
				<span className="font-mono text-[8px] text-(--f4)">
					{trace.transitionSource}
				</span>
			</div>

			<SearchCanvas trace={trace} />

			<div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-(--line) border-t px-3 py-1.5 font-mono text-[8px] text-(--f4)">
				<span className="flex items-center gap-1">
					<i className="inline-block h-1.5 w-1.5 rounded-full bg-(--f2)" />
					rolled out
				</span>
				<span className="flex items-center gap-1">
					<i className="inline-block h-1.5 w-1.5 rounded-full bg-(--acc) opacity-55" />
					pearl counterfactual
				</span>
				<span className="flex items-center gap-1">
					<i className="inline-block h-px w-2.5 bg-(--down)" />
					causally rejected
				</span>
				<span className="ml-auto">
					{trace.decisionUnavailable ? (
						<span className="text-(--down)">
							no estimable action · {trace.identificationStatus}
						</span>
					) : (
						<span>
							selected{" "}
							<span className="text-(--acc)">{trace.recommendedAction}</span> ·
							expected{" "}
							<span className="text-(--f2)">{fmt(trace.expectedOutcome)}</span>
							{" ± "}
							<span className="text-(--f3)">
								{fmt(trace.outcomeUncertainty, 3)}
							</span>
						</span>
					)}
				</span>
			</div>

			<BranchLedger trace={trace} />
		</div>
	);
};
