import React from "react";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";

export const RecognitionPanel = React.memo(() => (
	<Flex.Column className="gap-3 p-3 font-mono text-xs">
		{/* Precursor Cognition State */}
		<Section fit="content">
			<Section.Header
				title="Precursor recognition & associative memory"
				meta={<span data-l="recog-meta">0 frames · 0 learned situations</span>}
			/>
			<Section.Body scroll={false} className="p-3">
				<Flex.Row className="gap-6 flex-wrap">
					<Stat label="Confidence" value={<span data-metric="confidence" data-format="percent">0.0%</span>} />
					<Stat label="Contrast" value={<span data-metric="contrast" data-format="bits">0.00 bits</span>} />
					<Stat label="Ambiguity" value={<span data-metric="ambiguity" data-format="spread">Spread 0.000</span>} />
					<Stat label="Surprisal" value={<span data-metric="surprisal" data-format="surprisal">Surprisal 0.00 nat</span>} />
					<Stat label="Precursor Tokens" value={<span data-metric="precursor_length" data-format="integer">0</span>} />
					<Stat label="Trie Support" value={<span data-metric="support" data-format="integer">0</span>} />
					<Stat label="Resolved Fragments" value={<span data-metric="resolved" data-format="integer">0</span>} />
				</Flex.Row>
			</Section.Body>
			<Typography.Mono size="s" className="p-3 opacity-70" data-l="recog-status">
				Training precursor associations · Execution remains inert until confident
			</Typography.Mono>
		</Section>

		{/* Historical Held-Out Skill: Entry, Exit, and Economics */}
		<Section fit="content">
			<Section.Header
				title="Historical held-out recognition & economics"
				meta="Prequential scoring before model refinement"
			/>
			<Section.Body scroll={false} className="p-3">
				<div className="grid grid-cols-1 md:grid-cols-3 gap-4">
					{/* Entry Recognition */}
					<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-2">
						<div className="text-(--f2) font-bold uppercase tracking-wider text-[10px]">
							Held-Out Entry Recognition
						</div>
						<div className="flex flex-col gap-1 text-[11px]">
							<div className="flex justify-between">
								<span className="text-(--f4)">Valid UP Opportunities:</span>
								<span className="text-(--f1) font-bold" data-metric="hist_opportunities" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--up)">Correct ENTER at B:</span>
								<span className="text-(--up) font-bold" data-metric="hist_correct_enter" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--down)">Missed ENTER:</span>
								<span className="text-(--down) font-bold" data-metric="hist_missed_enter" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--down)">False ENTER (DOWN/UP_F/CHOP/FLAT):</span>
								<span className="text-(--down) font-bold" data-metric="hist_false_enter" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--f3)">Correct WAIT before B:</span>
								<span className="text-(--f2) font-bold" data-metric="hist_correct_wait" data-format="integer">0</span>
							</div>
						</div>
					</div>

					{/* Exit Recognition */}
					<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-2">
						<div className="text-(--f2) font-bold uppercase tracking-wider text-[10px]">
							Held-Out Exit Recognition
						</div>
						<div className="flex flex-col gap-1 text-[11px]">
							<div className="flex justify-between">
								<span className="text-(--up)">Correct EXIT at C:</span>
								<span className="text-(--up) font-bold" data-metric="hist_correct_exit" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--down)">Premature EXIT (before C):</span>
								<span className="text-(--down) font-bold" data-metric="hist_premature_exit" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--down)">Missed EXIT at C:</span>
								<span className="text-(--down) font-bold" data-metric="hist_missed_exit" data-format="integer">0</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--f3)">Holding WAIT (B → C):</span>
								<span className="text-(--f2) font-bold" data-metric="hist_correct_wait" data-format="integer">0</span>
							</div>
						</div>
					</div>

					{/* Historical Economics */}
					<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-2">
						<div className="text-(--f2) font-bold uppercase tracking-wider text-[10px]">
							Held-Out Executable Economics
						</div>
						<div className="flex flex-col gap-1 text-[11px]">
							<div className="flex justify-between">
								<span className="text-(--f4)">Mean Executable Return:</span>
								<span className="text-(--acc) font-bold" data-metric="hist_mean_return" data-format="insufficient_if_zero">—</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--f4)">Return Uncertainty (SE):</span>
								<span className="text-(--f2) font-bold" data-metric="hist_return_se" data-format="insufficient_if_zero">—</span>
							</div>
							<div className="flex justify-between">
								<span className="text-(--f4)">Lower Bound (Mean - SE):</span>
								<span className="text-(--f1) font-bold" data-metric="hist_lower_bound" data-format="insufficient_if_zero">—</span>
							</div>
							<div className="text-[10px] text-(--f4) mt-1">
								Liquidated at causal C minus entry cost at B. Never substitutes peak profit.
							</div>
						</div>
					</div>
				</div>
			</Section.Body>
		</Section>

		{/* Tape Fragment Classes & Pre-Outcome Prediction */}
		<Section fit="content">
			<Section.Header
				title="Empirical fragment classification & pre-outcome prediction"
				meta="Causal tape classification without human stories"
			/>
			<Section.Body scroll={false} className="p-3">
				<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
					{/* Fragment Counts */}
					<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-2">
						<div className="text-(--f2) font-bold uppercase tracking-wider text-[10px]">
							Completed Tape Fragments
						</div>
						<div className="grid grid-cols-6 gap-2 text-center text-[10px]">
							<div className="p-1.5 border border-(--line) rounded bg-(--surface)">
								<div className="text-(--up) font-bold">UP</div>
								<div className="text-sm font-bold mt-1" data-metric="fragments_up" data-format="integer">0</div>
							</div>
							<div className="p-1.5 border border-(--line) rounded bg-(--surface)">
								<div className="text-(--up) font-bold opacity-70">UP_F</div>
								<div className="text-sm font-bold mt-1" data-metric="fragments_up_friction" data-format="integer">0</div>
							</div>
							<div className="p-1.5 border border-(--line) rounded bg-(--surface)">
								<div className="text-(--down) font-bold">DOWN</div>
								<div className="text-sm font-bold mt-1" data-metric="fragments_down" data-format="integer">0</div>
							</div>
							<div className="p-1.5 border border-(--line) rounded bg-(--surface)">
								<div className="text-(--f3) font-bold">CHOP</div>
								<div className="text-sm font-bold mt-1" data-metric="fragments_chop" data-format="integer">0</div>
							</div>
							<div className="p-1.5 border border-(--line) rounded bg-(--surface)">
								<div className="text-(--f4) font-bold">FLAT</div>
								<div className="text-sm font-bold mt-1" data-metric="fragments_flat" data-format="integer">0</div>
							</div>
							<div className="p-1.5 border border-(--line) rounded bg-(--surface)">
								<div className="text-(--f4) font-bold">UNSUP</div>
								<div className="text-sm font-bold mt-1" data-metric="fragments_unsupported" data-format="integer">0</div>
							</div>
						</div>
					</div>

					{/* Pre-Outcome Frozen Prediction */}
					<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-2">
						<div className="text-(--f2) font-bold uppercase tracking-wider text-[10px]">
							Frozen Pre-Outcome Prediction vs Delayed Label
						</div>
						<div className="flex flex-col gap-1.5 text-[11px]">
							<div className="flex justify-between items-center">
								<span className="text-(--f4)">Frozen Predicted Action:</span>
								<span
									data-l="frozen-prediction"
									className="px-2 py-0.5 rounded text-[10px] font-bold bg-(--surface) border border-(--line) text-(--acc)"
								>
									<span data-metric="action" data-format="action">ABSTAIN</span>
								</span>
							</div>
							<div className="flex justify-between items-center">
								<span className="text-(--f4)">Actual Delayed Label:</span>
								<span
									data-l="delayed-label"
									className="px-2 py-0.5 rounded text-[10px] font-bold bg-(--surface) border border-(--line) text-(--f1)"
								>
									RESOLVING
								</span>
							</div>
							<div className="flex justify-between text-[10px] text-(--f4)">
								<span>Boundaries: A=<span data-metric="mark_a" data-format="integer">0</span> B=<span data-metric="mark_b" data-format="integer">0</span> C=<span data-metric="mark_c" data-format="integer">0</span></span>
								<span>Excursion: <span data-metric="excursion_mag" data-format="percent">0.0%</span></span>
							</div>
						</div>
					</div>
				</div>
			</Section.Body>
		</Section>
	</Flex.Column>
));
