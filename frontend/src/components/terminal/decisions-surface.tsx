import { useState } from "react";
import { strategyStore } from "#/collections/app";
import { DecisionChain } from "#/components/terminal/decision-chain";
import { DecisionSideRail } from "#/components/terminal/decision-side-rail";
import { LiveDecisionsEntryLine } from "#/components/terminal/decisions-entry-line";
import { Panel } from "#/components/ui/panel";
import { Decision } from "#/providers/telemetry/telemetry/decision";

type DecisionKey = {
	id: string;
	index: number;
};

const decObj = new Decision();

export const DecisionsSurface = () => {
	const [decisionKeys, setDecisionKeys] = useState<DecisionKey[]>([]);

	strategyStore.subscribe((state) => {
		const last = state.getLast();
		if (!last) return;

		const currentKeys: DecisionKey[] = [];
		for (let i = 0; i < last.decisionsLength(); i++) {
			const dec = last.decisions(i, decObj);
			const id = dec?.id() ?? String(i);
			currentKeys.push({ id, index: i });
		}

		if (currentKeys.map((k) => k.id).join(",") !== decisionKeys.map((k) => k.id).join(",")) {
			setDecisionKeys(currentKeys);
		}
	});

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-4.5">
				<LiveDecisionsEntryLine />

				<div className="mb-2 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Candidate evaluation
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						select a chain to scope causal + cognitive evidence
					</span>
				</div>

				<div className="flex flex-col gap-1.75">
					{decisionKeys.length === 0 ? (
						<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
							waiting for backend decision frames
						</Panel>
					) : (
						decisionKeys.map((key) => (
							<DecisionChain key={key.id} index={key.index} />
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

