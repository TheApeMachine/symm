import { createRef } from "react";
import type { CausalFrame } from "#/collections/types";
import {
	causalEntryBaseline,
	causalStrength,
	causalConfidence,
	latestCausalFrame,
} from "#/components/terminal/causal-view";
import { fixed } from "#/components/terminal/decision-format";
import { readDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Panel } from "@/components/ui/panel";

const entryPanelRef = createRef<HTMLDivElement>();
const entryLineRef = createRef<HTMLSpanElement>();
const entryStrengthRef = createRef<HTMLSpanElement>();
const entryConfidenceRef = createRef<HTMLSpanElement>();

const writeDecisionsEntryLine = (
	symbol: string | undefined,
	rows: CausalFrame[],
): void => {
	const visible = symbol !== undefined;

	if (entryPanelRef.current !== null) {
		entryPanelRef.current.style.display = visible ? "" : "none";
	}

	if (!visible) {
		return;
	}

	const frame = latestCausalFrame(rows, symbol);
	const entryLine = causalEntryBaseline(frame);
	const entryScore = causalStrength(frame);
	const entryConfidence = causalConfidence(frame);

	if (entryLineRef.current !== null) {
		entryLineRef.current.textContent = fixed(entryLine);
	}

	if (entryStrengthRef.current !== null) {
		entryStrengthRef.current.textContent = fixed(entryScore);
	}

	if (entryConfidenceRef.current !== null) {
		entryConfidenceRef.current.textContent = fixed(entryConfidence);
	}
};

/*
paintDecisionsEntryLine paints the selected candidate entry gate from the
current DRAW causal batch. Unmatched batches leave the prior gate values.
*/
export const paintDecisionsEntryLine = (
	value: unknown,
	_focusSymbol: string,
) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as CausalFrame[];

	const scope = readDecisionsScopeSymbol();

	if (scope === undefined) {
		writeDecisionsEntryLine(undefined, []);
		return;
	}

	if (entryPanelRef.current !== null) {
		entryPanelRef.current.style.display = "";
	}

	for (let index = rows.length - 1; index >= 0; index -= 1) {
		const frame = rows[index];

		if (frame?.symbol === scope) {
			writeDecisionsEntryLine(scope, [frame]);
			return;
		}
	}
};

/*
LiveDecisionsEntryLine is the static entry-gate shell. DRAW paints via
paintDecisionsEntryLine.
*/
export const LiveDecisionsEntryLine = () => (
	<div ref={entryPanelRef} style={{ display: "none" }}>
			<Panel className="mb-3.5 flex items-center gap-3.5 px-3 py-2 font-mono text-[11.5px]">
				<span className="text-(--f3)">entry line</span>
				<span ref={entryLineRef} className="font-semibold text-(--acc)" />
				<span className="text-(--f4)">·</span>
				<span className="text-(--f3)">
					strength <span ref={entryStrengthRef} />
				</span>
				<span className="text-(--f4)">·</span>
				<span className="text-(--f3)">
					confidence <span ref={entryConfidenceRef} />
				</span>
				<span className="ml-auto text-(--f4)">
					support gate ≥ 2 · strategy utility wins
				</span>
			</Panel>
		</div>
);
