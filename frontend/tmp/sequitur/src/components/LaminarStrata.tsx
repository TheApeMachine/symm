import type React from "react";
import { type GrammarSnapshot, RULE_PALETTE } from "../engine/sequitur";

interface LaminarStrataProps {
	snapshot: GrammarSnapshot;
	hoveredRuleId: string | null;
	onHoverRule: (ruleId: string | null) => void;
	hoveredCharIndex: number | null;
	onHoverChar: (charIndex: number | null) => void;
}

export const LaminarStrata: React.FC<LaminarStrataProps> = ({
	snapshot,
	hoveredRuleId,
	onHoverRule,
	hoveredCharIndex,
	onHoverChar,
}) => {
	const fullChars = snapshot.fullInput.split("");
	const processedCount = Math.max(0, snapshot.charIndex + 1);

	// Group active rules by their structural depth (or list each rule as its own horizontal track)
	// Rendering a dedicated horizontal track for S (Top Canopy) + each active rule R1..Rn ordered by depth
	// makes overlapping or nested occurrences crystal-clear on the character grid!
	const sortedRulesByDepth = [...snapshot.rules].sort((a, b) => {
		if (b.depth !== a.depth) return b.depth - a.depth;
		return a.number - b.number;
	});

	return (
		<section
			id="laminar-strata"
			className="bg-white border border-slate-200 rounded-xl p-5 shadow-2xs"
		>
			<div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-100">
				<div>
					<div className="flex items-center gap-2 text-xs text-slate-500 font-medium">
						<span>02. Spatial Alignment Lens</span>
						<span aria-hidden="true">·</span>
						<span>Laminar Compression Strata</span>
						<span aria-hidden="true">·</span>
						<span className="font-mono tabular-nums">
							{snapshot.rules.length} active rule tracks
						</span>
					</div>
					<h2 className="text-lg font-semibold text-slate-900 mt-0.5">
						Multi-Layer Character Span Alignment
					</h2>
				</div>

				<div className="flex items-center gap-3 text-xs text-slate-500">
					<span>
						Higher strata represent rules of greater compositional depth
					</span>
					<span aria-hidden="true">·</span>
					<span className="font-mono text-slate-700 tabular-nums">
						Max Depth: {snapshot.rootRule.depth}
					</span>
				</div>
			</div>

			{/* Scrollable Horizontal Stratum Grid */}
			<div className="mt-4 overflow-x-auto pb-2">
				<div
					className="min-w-[640px]"
					style={{
						width: `${Math.max(100, fullChars.length * 42 + 160)}px`,
					}}
				>
					{/* Track 1: Top-Level S Partition Canopy */}
					<div className="grid grid-cols-[140px_1fr] items-center gap-3 py-2 border-b border-slate-100">
						<div className="flex flex-col pr-2">
							<span className="font-mono text-xs font-bold text-slate-900">
								S Canopy
							</span>
							<span className="text-[11px] text-slate-500">
								Top-level tiling
							</span>
						</div>

						<div
							className="relative grid gap-1 h-9 items-center"
							style={{
								gridTemplateColumns: `repeat(${Math.max(1, fullChars.length)}, minmax(0, 1fr))`,
							}}
						>
							{snapshot.rootRule.symbols.map((tok) => {
								if (!tok.charSpan) return null;
								const [start, end] = tok.charSpan;
								const ruleObj = tok.ruleId
									? snapshot.rules.find((r) => r.id === tok.ruleId)
									: undefined;
								const palette = ruleObj
									? RULE_PALETTE[ruleObj.colorIndex]
									: null;
								const isHovered =
									(tok.ruleId && hoveredRuleId === tok.ruleId) ||
									(hoveredCharIndex !== null &&
										hoveredCharIndex >= start &&
										hoveredCharIndex <= end);

								return (
									<div
										key={tok.id}
										onMouseEnter={() => {
											if (tok.ruleId) onHoverRule(tok.ruleId);
											else onHoverChar(start);
										}}
										onMouseLeave={() => {
											onHoverRule(null);
											onHoverChar(null);
										}}
										style={{
											gridColumn: `${start + 1} / ${end + 2}`,
										}}
										className={`h-7 rounded-md border flex items-center justify-center px-1.5 font-mono text-xs font-semibold transition-transform duration-150 cursor-default truncate ${
											palette
												? `${palette.bg} ${palette.border} ${palette.text}`
												: "bg-slate-100 border-slate-300 text-slate-700"
										} ${isHovered ? "-translate-y-0.5 ring-2 ring-[#0284C7]" : ""}`}
										title={
											ruleObj
												? `${ruleObj.id} → "${ruleObj.expansion}" (chars ${start + 1}..${end + 1})`
												: `Terminal '${tok.value}' (char ${start + 1})`
										}
									>
										<span>{tok.value}</span>
										{ruleObj && end > start && (
											<span className="ml-1.5 text-[10px] opacity-75 font-normal truncate">
												“{ruleObj.expansion}”
											</span>
										)}
									</div>
								);
							})}
						</div>
					</div>

					{/* Tracks for Each Active Rule (ordered from highest depth to depth 1) */}
					{sortedRulesByDepth.length === 0 ? (
						<div className="py-6 text-center text-xs text-slate-400 font-mono border-b border-slate-100">
							No composite rules minted yet — strata form automatically as
							digrams repeat.
						</div>
					) : (
						sortedRulesByDepth.map((rule) => {
							const palette = RULE_PALETTE[rule.colorIndex];
							const isRuleActive =
								hoveredRuleId === rule.id ||
								snapshot.highlightRuleId === rule.id;

							return (
								<div
									key={rule.id}
									onMouseEnter={() => onHoverRule(rule.id)}
									onMouseLeave={() => onHoverRule(null)}
									className={`grid grid-cols-[140px_1fr] items-center gap-3 py-2 border-b border-slate-100 transition-colors ${
										isRuleActive ? "bg-slate-50/90" : "hover:bg-slate-50/50"
									}`}
								>
									{/* Left Track Label */}
									<div className="flex items-center justify-between pr-2">
										<div className="flex flex-col">
											<div className="flex items-center gap-1.5">
												<span
													className={`font-mono text-xs font-bold ${palette.text}`}
												>
													{rule.id}
												</span>
												<span className="text-[11px] font-mono text-slate-400">
													→ {rule.symbols.map((s) => s.value).join(" ")}
												</span>
											</div>
											<span className="text-[10px] font-mono text-slate-500 tabular-nums">
												Depth {rule.depth} · {rule.refCount}× utility
											</span>
										</div>
									</div>

									{/* Right Character Span Bars */}
									<div
										className="relative grid gap-1 h-8 items-center"
										style={{
											gridTemplateColumns: `repeat(${Math.max(1, fullChars.length)}, minmax(0, 1fr))`,
										}}
									>
										{/* Subtle vertical column guides */}
										{fullChars.map((_, cIdx) => (
											<div
												key={`guide-${cIdx}`}
												style={{
													gridColumn: `${cIdx + 1} / ${cIdx + 2}`,
													gridRow: 1,
												}}
												className={`h-full border-r border-dashed ${
													hoveredCharIndex === cIdx
														? "bg-slate-200/50 border-slate-300"
														: "border-slate-100"
												}`}
											/>
										))}

										{/* Rule Occurrence Span Blocks */}
										{rule.intervals.map(([start, end], occIdx) => {
											const isCharMatch =
												hoveredCharIndex !== null &&
												hoveredCharIndex >= start &&
												hoveredCharIndex <= end;

											return (
												<div
													key={`${rule.id}-occ-${occIdx}-${start}`}
													style={{
														gridColumn: `${start + 1} / ${end + 2}`,
														gridRow: 1,
													}}
													className={`z-10 h-6 rounded border flex items-center justify-between px-2 font-mono text-[11px] font-semibold transition-transform duration-150 ${
														palette.bg
													} ${palette.border} ${palette.text} ${
														isRuleActive || isCharMatch
															? "ring-2 ring-[#0284C7] -translate-y-0.5 shadow-2xs"
															: ""
													}`}
													title={`${rule.id} occurrence #${occIdx + 1}: "${
														rule.expansion
													}" spanning chars ${start + 1}..${end + 1}`}
												>
													<span>{rule.id}</span>
													<span className="text-[10px] opacity-80 truncate ml-1">
														{rule.expansion}
													</span>
												</div>
											);
										})}
									</div>
								</div>
							);
						})
					)}

					{/* Base Bedrock Row: Raw Character Stream Ruler */}
					<div className="grid grid-cols-[140px_1fr] items-center gap-3 pt-2.5">
						<div className="flex flex-col pr-2">
							<span className="font-mono text-xs font-semibold text-slate-700">
								Terminal Stream
							</span>
							<span className="text-[10px] font-mono text-slate-400 tabular-nums">
								1..{fullChars.length} index ruler
							</span>
						</div>

						<div
							className="grid gap-1"
							style={{
								gridTemplateColumns: `repeat(${Math.max(1, fullChars.length)}, minmax(0, 1fr))`,
							}}
						>
							{fullChars.map((ch, idx) => {
								const isProcessed = idx < processedCount;
								const isHovered = hoveredCharIndex === idx;
								return (
									<div
										key={`bedrock-${idx}`}
										onMouseEnter={() => onHoverChar(idx)}
										onMouseLeave={() => onHoverChar(null)}
										className={`flex flex-col items-center justify-center py-1 rounded font-mono text-xs transition-colors cursor-default ${
											isHovered
												? "bg-slate-900 text-white font-bold"
												: isProcessed
													? "bg-slate-100 text-slate-800 font-medium"
													: "bg-slate-50 text-slate-300"
										}`}
									>
										<span>{ch}</span>
										<span className="text-[9px] opacity-60 tabular-nums">
											{idx + 1}
										</span>
									</div>
								);
							})}
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};
