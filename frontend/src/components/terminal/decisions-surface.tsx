import { useState } from "react";
import { strategyStore } from "#/collections/app";
import { DecisionChain } from "#/components/terminal/decision-chain";
import { DecisionSideRail } from "#/components/terminal/decision-side-rail";
import { Panel } from "#/components/ui/panel";
import { Decision } from "#/providers/telemetry/telemetry/decision";

/*
The surface holds one row per symbol, not one row per frame.

Each strategy frame carries the decisions for whichever symbols were evaluated
on that tick, which is usually a single symbol. Mirroring only the newest frame
therefore replaced the visible row every tick, so a decision appeared and was
immediately overwritten by the next symbol's. Scanning the retained frames and
keeping the most recent decision per symbol is what makes the surface a board
of live candidates rather than a one-slot flicker.
*/
type DecisionKey = {
	symbol: string;
	frame: number;
	index: number;
};

/*
FRAME_SCAN bounds how far back a rebuild looks. The store retains 50 frames;
scanning them keeps a symbol on screen for its full retention window rather
than dropping it the moment another symbol is decided.
*/
const FRAME_SCAN = 50;

const collectKeys = (): DecisionKey[] => {
	const frames = strategyStore.state.getLastN(FRAME_SCAN);
	const latest = new Map<string, DecisionKey>();

	for (let position = 0; position < frames.length; position += 1) {
		const frame = frames[position];

		if (!frame) {
			continue;
		}

		for (let index = 0; index < frame.decisionsLength(); index += 1) {
			// A fresh accessor per read: the flatbuffer reader mutates and
			// returns the same instance when one is supplied, so a shared
			// object would leave every map entry pointing at the last read.
			const decision = frame.decisions(index, new Decision());
			const symbol = decision?.symbol() ?? "";

			if (symbol === "") {
				continue;
			}

			// Later frames overwrite earlier ones for the same symbol, so the
			// map ends holding each symbol's most recent decision.
			latest.set(symbol, { symbol, frame: position, index });
		}
	}

	return [...latest.values()].sort((left, right) =>
		left.symbol.localeCompare(right.symbol),
	);
};

export const DecisionsSurface = () => {
	const [decisionKeys, setDecisionKeys] = useState<DecisionKey[]>(collectKeys);

	strategyStore.subscribe(() => {
		const next = collectKeys();
		const signature = next
			.map((key) => `${key.symbol}:${key.frame}:${key.index}`)
			.join(",");
		const current = decisionKeys
			.map((key) => `${key.symbol}:${key.frame}:${key.index}`)
			.join(",");

		if (signature !== current) {
			setDecisionKeys(next);
		}
	});

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-4.5">
				<div className="mb-2 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Candidate evaluation
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						{decisionKeys.length === 0
							? "select a chain to scope causal + cognitive evidence"
							: `${decisionKeys.length} symbols evaluated · click a row to expand`}
					</span>
				</div>

				<div className="flex flex-col gap-1.75">
					{decisionKeys.length === 0 ? (
						<Panel
							variant="surface"
							size="bare"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for backend decision frames
						</Panel>
					) : (
						decisionKeys.map((key) => (
							<DecisionChain
								key={key.symbol}
								frame={key.frame}
								index={key.index}
							/>
						))
					)}
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<DecisionSideRail />
			</div>
		</div>
	);
};
