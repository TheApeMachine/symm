import { Layers } from "lucide-react";
import React, { useState } from "react";
import {
	buildNestedCapsules,
	type GrammarSnapshot,
	type NestedCapsuleNode,
	RULE_PALETTE,
} from "../engine/sequitur";

interface TelescopingTapeProps {
	snapshot: GrammarSnapshot;
	hoveredRuleId: string | null;
	onHoverRule: (ruleId: string | null) => void;
	hoveredCharIndex: number | null;
	onHoverChar: (charIndex: number | null) => void;
	hoveredDigramKey: string | null;
	onHoverDigram: (key: string | null) => void;
}

export const TelescopingTape: React.FC<TelescopingTapeProps> = ({
	snapshot,
	hoveredRuleId,
	onHoverRule,
	hoveredCharIndex,
	onHoverChar,
	hoveredDigramKey,
	onHoverDigram,
}) => {
	const maxGrammarDepth = Math.max(1, snapshot.rootRule.depth);
	// Default to full X-ray depth (up to 4) so users immediately see nested capsules
	const [unfurlDepth, setUnfurlDepth] = useState<number>(3);
	// Individual capsule override toggles (by instanceId)
	const [toggledCapsules, setToggledCapsules] = useState<
		Record<string, boolean>
	>({});

	const nestedNodes = buildNestedCapsules(snapshot);

	const toggleCapsuleInstance = (instanceId: string, e: React.MouseEvent) => {
		e.stopPropagation();
		setToggledCapsules((prev) => ({
			...prev,
			[instanceId]: !prev[instanceId],
		}));
	};

	const isCharRangeHovered = (start: number, end: number): boolean => {
		if (hoveredCharIndex === null) return false;
		return hoveredCharIndex >= start && hoveredCharIndex <= end;
	};

	const renderCapsuleNode = (
		node: NestedCapsuleNode,
		currentLevel: number,
	): React.ReactNode => {
		const charHovered = isCharRangeHovered(node.startChar, node.endChar);

		if (!node.isRule || !node.ruleId) {
			// Terminal symbol tile
			const isHighlightedChar = hoveredCharIndex === node.startChar;
			const isLatestAppended =
				snapshot.kind === "APPEND_SYMBOL" &&
				node.startChar === snapshot.charIndex;

			return (
				<div
					key={node.instanceId}
					onMouseEnter={() => onHoverChar(node.startChar)}
					onMouseLeave={() => onHoverChar(null)}
					className={`group relative inline-flex flex-col items-center justify-center min-w-[34px] h-[42px] px-2 rounded-md border transition-transform duration-150 select-none cursor-default ${
						isLatestAppended
							? "bg-[#E0F2FE] border-[#0284C7] text-[#0369A1] font-semibold -translate-y-0.5 shadow-xs"
							: isHighlightedChar
								? "bg-[#0F172A] border-[#0F172A] text-white -translate-y-0.5"
								: charHovered
									? "bg-slate-100 border-slate-400 text-slate-900"
									: "bg-white border-slate-200/90 text-slate-800 hover:border-slate-400"
					}`}
					title={`Terminal '${node.symbol}' at stream position ${node.startChar + 1}`}
				>
					<span className="font-mono text-sm font-semibold leading-none">
						{node.symbol}
					</span>
					<span
						className={`font-mono text-[10px] leading-none mt-1 tabular-nums ${
							isHighlightedChar ? "text-slate-300" : "text-slate-400"
						}`}
					>
						{node.startChar + 1}
					</span>
				</div>
			);
		}

		// Rule capsule
		const palette = RULE_PALETTE[node.colorIndex ?? 0];
		const isRuleHovered =
			hoveredRuleId === node.ruleId || snapshot.highlightRuleId === node.ruleId;

		// Determine whether this capsule is expanded into its inner children
		const defaultExpanded = currentLevel < unfurlDepth;
		const isExpanded =
			toggledCapsules[node.instanceId] !== undefined
				? toggledCapsules[node.instanceId]
				: defaultExpanded;

		return (
			<div
				key={node.instanceId}
				onMouseEnter={(e) => {
					e.stopPropagation();
					onHoverRule(node.ruleId!);
				}}
				onMouseLeave={(e) => {
					e.stopPropagation();
					onHoverRule(null);
				}}
				onClick={(e) => toggleCapsuleInstance(node.instanceId, e)}
				className={`relative inline-flex items-center rounded-lg border transition-transform duration-150 cursor-pointer select-none ${
					palette.border
				} ${isExpanded ? palette.bgSubtle : palette.bg} ${
					isRuleHovered
						? "ring-2 ring-[#0284C7] -translate-y-0.5 shadow-sm"
						: charHovered
							? "ring-1 ring-slate-400"
							: ""
				} ${isExpanded ? "p-1.5 gap-1.5" : "px-2.5 py-1.5 gap-2"}`}
				title={`Click to ${isExpanded ? "collapse" : "unfurl"} ${node.ruleId} ("${
					node.expansion
				}", chars ${node.startChar + 1}–${node.endChar + 1})`}
			>
				{/* Rule Capsule Header Tag */}
				<div className="flex flex-col items-start justify-center px-1">
					<div className="flex items-center gap-1">
						<span
							className={`font-mono text-xs font-bold tracking-tight ${palette.text}`}
						>
							{node.ruleId}
						</span>
						<span className="text-[10px] font-mono text-slate-500 tabular-nums">
							[{node.startChar + 1}..{node.endChar + 1}]
						</span>
					</div>
					{!isExpanded && (
						<span className="font-mono text-[11px] text-slate-600 tracking-wider mt-0.5">
							“{node.expansion}”
						</span>
					)}
				</div>

				{/* If expanded, render nested children inside this Russian-doll capsule */}
				{isExpanded && node.children && (
					<div className="inline-flex items-center gap-1 pl-1.5 border-l border-slate-300/80">
						{node.children.map((child) =>
							renderCapsuleNode(child, currentLevel + 1),
						)}
					</div>
				)}
			</div>
		);
	};

	// Check if any root adjacent pair matches highlightDigram or hoveredDigramKey
	const rootSymbols = snapshot.rootRule.symbols;

	return (
		<section
			id="telescoping-tape"
			className="bg-white border border-slate-200 rounded-xl p-5 shadow-2xs"
		>
			{/* Header Row */}
			<div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-100">
				<div>
					<div className="flex items-center gap-2 text-xs text-slate-500 font-medium">
						<span>01. Primary Grammar Lens</span>
						<span aria-hidden="true">·</span>
						<span>Telescoping Nested Capsule Ribbon</span>
						<span aria-hidden="true">·</span>
						<span className="font-mono tabular-nums">
							|S| = {rootSymbols.length} top-level symbols
						</span>
					</div>
					<h2 className="text-lg font-semibold text-slate-900 mt-0.5">
						Root Sequence <code className="font-mono text-[#0284C7]">S</code> as
						Inline Nested Capsules
					</h2>
				</div>

				{/* Unfurl Depth Slider Control */}
				<div className="flex items-center gap-3 bg-slate-50 border border-slate-200/80 rounded-lg px-3 py-1.5">
					<label
						htmlFor="unfurl-depth-slider"
						className="flex items-center gap-1.5 text-xs font-medium text-slate-700 whitespace-nowrap shrink-0"
					>
						<Layers className="w-3.5 h-3.5 text-[#0284C7]" />
						<span>Unfurl Depth:</span>
						<span className="font-mono font-semibold text-[#0284C7] tabular-nums">
							{unfurlDepth === 0
								? "0 (Compact)"
								: unfurlDepth >= maxGrammarDepth
									? `${unfurlDepth} (Full X-Ray)`
									: `Level ${unfurlDepth}`}
						</span>
					</label>
					<input
						id="unfurl-depth-slider"
						type="range"
						min={0}
						max={Math.max(3, maxGrammarDepth)}
						step={1}
						value={Math.min(unfurlDepth, Math.max(3, maxGrammarDepth))}
						onChange={(e) => {
							setUnfurlDepth(Number(e.target.value));
							setToggledCapsules({});
						}}
						className="w-24 accent-[#0284C7] cursor-pointer"
					/>
					<button
						type="button"
						onClick={() => {
							setUnfurlDepth((prev) =>
								prev > 0 ? 0 : Math.max(3, maxGrammarDepth),
							);
							setToggledCapsules({});
						}}
						className="px-2 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-100 transition-colors whitespace-nowrap shrink-0"
					>
						{unfurlDepth > 0 ? "Collapse All" : "Expand All"}
					</button>
				</div>
			</div>

			{/* Active Micro-Step Banner */}
			<div
				className={`mt-4 px-3.5 py-2.5 rounded-lg border flex flex-wrap items-center justify-between gap-3 text-xs ${
					snapshot.kind === "DIGRAM_COLLISION"
						? "bg-[#FEF2F2] border-[#FECACA] text-[#991B1B]"
						: snapshot.kind === "DISSOLVE_UNDERUSED_RULE"
							? "bg-[#FFFBEB] border-[#FDE68A] text-[#92400E]"
							: snapshot.kind === "CREATE_NEW_RULE" ||
									snapshot.kind === "REUSE_EXISTING_RULE"
								? "bg-[#F0FDF4] border-[#BBF7D0] text-[#166534]"
								: "bg-slate-50 border-slate-200/80 text-slate-700"
				}`}
			>
				<div className="flex items-center gap-2.5">
					<span className="font-mono font-semibold uppercase tracking-wider px-1.5 py-0.5 rounded bg-white/80 border border-current/20 whitespace-nowrap shrink-0">
						{snapshot.kind === "DIGRAM_COLLISION"
							? "▲ Collision Detected"
							: snapshot.kind === "CREATE_NEW_RULE"
								? "◆ Rule Minted"
								: snapshot.kind === "REUSE_EXISTING_RULE"
									? "◆ Rule Reused"
									: snapshot.kind === "DISSOLVE_UNDERUSED_RULE"
										? "▲ Utility Dissolution"
										: snapshot.kind === "APPEND_SYMBOL"
											? "● Symbol Appended"
											: "● Ready"}
					</span>
					<span className="font-semibold text-sm">{snapshot.title}</span>
					<span className="hidden sm:inline text-slate-400" aria-hidden="true">
						—
					</span>
					<span className="text-xs opacity-90">{snapshot.explanation}</span>
				</div>

				<span className="font-mono text-[11px] opacity-75 whitespace-nowrap shrink-0 tabular-nums">
					Click any rule capsule below to toggle inline inspection
				</span>
			</div>

			{/* Main Horizontal Telescoping Capsule Stream */}
			<div className="mt-4 pt-2 pb-3 overflow-x-auto">
				{nestedNodes.length === 0 ? (
					<div className="py-10 text-center text-sm text-slate-400 font-mono">
						Sequence S is empty — advance playback or click a character above to
						begin.
					</div>
				) : (
					<div className="inline-flex items-center gap-2 min-w-full py-2 px-1">
						{/* Root S Label */}
						<div className="inline-flex items-center gap-1.5 pr-2 border-r border-slate-200 shrink-0">
							<span className="font-mono text-sm font-bold text-slate-900">
								S
							</span>
							<span className="font-mono text-xs text-slate-400">→</span>
						</div>

						{/* Capsule Nodes with interactive Digram Bridges between adjacent top-level symbols */}
						{nestedNodes.map((node, idx) => {
							const nextNode =
								idx < nestedNodes.length - 1 ? nestedNodes[idx + 1] : null;
							const digramKey = nextNode
								? `${node.symbol}|${nextNode.symbol}`
								: null;
							const isHighlightedDigram =
								(snapshot.highlightDigram &&
									nextNode &&
									snapshot.highlightDigram[0] === node.symbol &&
									snapshot.highlightDigram[1] === nextNode.symbol) ||
								(digramKey !== null && hoveredDigramKey === digramKey);

							return (
								<React.Fragment key={node.instanceId}>
									{renderCapsuleNode(node, 0)}

									{/* Adjacent Digram Connector Bridge */}
									{nextNode && digramKey && (
										<button
											type="button"
											onMouseEnter={() => onHoverDigram(digramKey)}
											onMouseLeave={() => onHoverDigram(null)}
											onClick={() =>
												onHoverDigram(
													hoveredDigramKey === digramKey ? null : digramKey,
												)
											}
											title={`Digram [${node.symbol} ${nextNode.symbol}] in S at pos ${idx + 1}`}
											className={`group relative flex items-center justify-center w-4 h-6 rounded transition-colors shrink-0 ${
												isHighlightedDigram
													? "bg-[#DC2626] text-white shadow-xs"
													: "text-slate-300 hover:text-slate-600 hover:bg-slate-100"
											}`}
										>
											<span className="font-mono text-[10px] leading-none">
												{isHighlightedDigram ? "⚡" : "·"}
											</span>
										</button>
									)}
								</React.Fragment>
							);
						})}
					</div>
				)}
			</div>

			{/* Synchronized Raw Input Stream Footer Bar */}
			<div className="mt-2 pt-3 border-t border-slate-100 flex flex-wrap items-center justify-between gap-4">
				<div className="flex items-center gap-2 overflow-x-auto py-1">
					<span className="text-xs font-medium text-slate-500 whitespace-nowrap shrink-0 mr-1">
						Raw Input Stream:
					</span>
					<div className="inline-flex items-center gap-1">
						{snapshot.fullInput.split("").map((ch, idx) => {
							const isProcessed = idx <= snapshot.charIndex;
							const isCurrent = idx === snapshot.charIndex;
							const isHovered = hoveredCharIndex === idx;

							// Check if hoveredRuleId covers this character index
							const activeHoverRule = snapshot.rules.find(
								(r) => r.id === hoveredRuleId,
							);
							const isCoveredByHoveredRule =
								activeHoverRule?.intervals.some(
									([start, end]) => idx >= start && idx <= end,
								) ?? false;

							return (
								<button
									key={idx}
									type="button"
									onMouseEnter={() => onHoverChar(idx)}
									onMouseLeave={() => onHoverChar(null)}
									className={`flex flex-col items-center justify-center w-6 h-8 rounded font-mono text-xs transition-transform duration-150 shrink-0 ${
										isHovered
											? "bg-slate-900 text-white -translate-y-0.5 font-bold"
											: isCoveredByHoveredRule
												? "bg-[#0284C7] text-white font-semibold -translate-y-0.5"
												: isCurrent
													? "bg-[#E0F2FE] text-[#0369A1] border border-[#0284C7] font-bold"
													: isProcessed
														? "bg-slate-100 text-slate-800 border border-slate-200/80"
														: "bg-slate-50/60 text-slate-300 border border-dashed border-slate-200"
									}`}
									title={`Stream index ${idx + 1}: '${ch}' (${
										isProcessed ? "Processed" : "Upcoming"
									})`}
								>
									<span className="leading-none">{ch}</span>
									<span className="text-[9px] opacity-65 leading-none mt-0.5 tabular-nums">
										{idx + 1}
									</span>
								</button>
							);
						})}
					</div>
				</div>

				<div className="flex items-center gap-3 text-xs text-slate-500 font-mono tabular-nums">
					<span>
						Processed: {Math.max(0, snapshot.charIndex + 1)} /{" "}
						{snapshot.fullInput.length} chars
					</span>
					<span aria-hidden="true">·</span>
					<span>Grammar Size: {snapshot.metrics.grammarSymbols} symbols</span>
				</div>
			</div>
		</section>
	);
};
