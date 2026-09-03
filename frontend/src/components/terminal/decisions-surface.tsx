import { useSelector } from "@tanstack/react-store";
import { decisionStore } from "#/collections/app";
import { DecisionChain } from "#/components/terminal/decision-chain";
import { DecisionSideRail } from "#/components/terminal/decision-side-rail";
import { Panel } from "#/components/ui/panel";

/*
The surface holds one row per symbol.

Rows are addressed by symbol, which is the identity the board is actually about
— exactly one live decision per symbol, replaced whenever that symbol is
re-evaluated. The previous version reconstructed that by scanning a 50-frame
ring and de-duplicating, which meant a row's identity was a position in a
rotating buffer: once the ring filled, every new frame shifted all positions and
rows drifted onto other symbols' data while the list reshuffled underneath an
open row. There is no position to drift now.
*/

export const DecisionsSurface = () => {
	const symbols = useSelector(decisionStore, (state) =>
		Object.keys(state.bySymbol).sort(),
	);

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-4.5">
				<div className="mb-2 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Candidate evaluation
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						{symbols.length === 0
							? "select a chain to scope causal + cognitive evidence"
							: `${symbols.length} symbols evaluated · click a row to expand`}
					</span>
				</div>

				<div className="flex flex-col gap-1.75">
					{symbols.length === 0 ? (
						<Panel
							variant="surface"
							size="bare"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for backend decision frames
						</Panel>
					) : (
						symbols.map((symbol) => (
							<DecisionChain key={symbol} symbol={symbol} />
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
