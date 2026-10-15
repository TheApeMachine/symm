import { Layers, Maximize2, Minimize2 } from "lucide-react";
import { useState } from "react";
import { Button, Section, Typography } from "#/components/ui";
import {
	buildNestedCapsules,
	type GrammarSnapshot,
	type NestedCapsuleNode,
	RULE_PALETTE,
} from "./sequitur-engine";

interface SequiturTelescopingTapeProps {
	snapshot: GrammarSnapshot;
	hoveredRuleId: string | null;
	onHoverRule: (ruleId: string | null) => void;
	hoveredCharIndex: number | null;
	onHoverChar: (charIndex: number | null) => void;
	hoveredDigramKey: string | null;
	onHoverDigram: (key: string | null) => void;
}

export const SequiturTelescopingTape = ({
	snapshot,
	hoveredRuleId,
	onHoverRule,
	hoveredCharIndex,
	onHoverChar,
}: SequiturTelescopingTapeProps) => {
	const maxGrammarDepth = Math.max(1, snapshot.rootRule.depth);
	const [unfurlDepth, setUnfurlDepth] = useState<number>(3);
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
			const isHighlightedChar = hoveredCharIndex === node.startChar;
			const isLatestAppended =
				snapshot.kind === "APPEND_SYMBOL" &&
				node.startChar === snapshot.charIndex;

			return (
				<button
					type="button"
					key={node.instanceId}
					onMouseEnter={() => onHoverChar(node.startChar)}
					onMouseLeave={() => onHoverChar(null)}
					onFocus={() => onHoverChar(node.startChar)}
					onBlur={() => onHoverChar(null)}
					className={`group relative inline-flex flex-col items-center justify-center min-w-[32px] h-[34px] px-1.5 rounded-[2px] border transition-colors select-none font-mono ${
						isLatestAppended
							? "bg-sky-950/80 border-sky-400 text-sky-200 font-bold"
							: isHighlightedChar
								? "bg-(--raised) border-(--f1) text-(--f1)"
								: charHovered
									? "bg-(--raised) border-(--f3) text-(--f1)"
									: "bg-(--surface) border-(--line) text-(--f2) hover:border-(--line2)"
					}`}
					title={`Terminal '${node.symbol}' (pos ${node.startChar + 1})`}
				>
					<span className="text-[11px] font-semibold leading-none">
						{node.symbol}
					</span>
					<span
						className={`text-[8.5px] leading-none mt-0.5 tabular-nums ${
							isHighlightedChar ? "text-(--acc)" : "text-(--f4)"
						}`}
					>
						{node.startChar + 1}
					</span>
				</button>
			);
		}

		const palette = RULE_PALETTE[node.colorIndex ?? 0];
		const isRuleHovered =
			hoveredRuleId === node.ruleId || snapshot.highlightRuleId === node.ruleId;

		const defaultExpanded = currentLevel < unfurlDepth;
		const isExpanded =
			toggledCapsules[node.instanceId] !== undefined
				? toggledCapsules[node.instanceId]
				: defaultExpanded;

		return (
			<div
				key={node.instanceId}
				className={`relative inline-flex items-center rounded-[2px] border transition-colors select-none font-mono ${
					palette.border
				} ${isExpanded ? palette.bgSubtle : palette.bg} ${
					isRuleHovered
						? "ring-1 ring-sky-400"
						: charHovered
							? "ring-1 ring-(--f3)"
							: ""
				} ${isExpanded ? "p-1 gap-1" : "px-2 py-1 gap-1.5"}`}
			>
				{/* Rule Capsule Header Toggle */}
				<button
					type="button"
					onClick={(e) => toggleCapsuleInstance(node.instanceId, e)}
					onMouseEnter={(e) => {
						e.stopPropagation();
						if (node.ruleId) onHoverRule(node.ruleId);
					}}
					onMouseLeave={(e) => {
						e.stopPropagation();
						onHoverRule(null);
					}}
					onFocus={() => {
						if (node.ruleId) onHoverRule(node.ruleId);
					}}
					onBlur={() => {
						onHoverRule(null);
					}}
					className="flex flex-col items-start justify-center cursor-pointer text-left bg-transparent border-none p-0 outline-none select-none"
					title={`Click to ${isExpanded ? "collapse" : "unfurl"} ${node.ruleId} ("${
						node.expansion
					}", tokens ${node.startChar + 1}–${node.endChar + 1})`}
				>
					<div className="flex items-center gap-1">
						<span
							className={`text-[11px] font-bold tracking-tight ${palette.text}`}
						>
							{node.ruleId}
						</span>
						<span className="text-[8.5px] text-(--f4) tabular-nums">
							[{node.startChar + 1}..{node.endChar + 1}]
						</span>
					</div>
					{!isExpanded && (
						<span className="text-[9.5px] text-(--f3) tracking-wider whitespace-nowrap">
							“{node.expansion}”
						</span>
					)}
				</button>

				{/* Expanded children */}
				{isExpanded && node.children && (
					<div className="inline-flex items-center gap-1 pl-1 border-l border-(--line2)">
						{node.children.map((child) =>
							renderCapsuleNode(child, currentLevel + 1),
						)}
					</div>
				)}
			</div>
		);
	};

	const rootSymbols = snapshot.rootRule.symbols;

	return (
		<Section fit="pane" className="border-b border-(--line) bg-(--surface)">
			<Section.Header
				size="s"
				title="Telescoping Tape"
				meta={`|S| = ${rootSymbols.length} tokens`}
			>
				{/* Unfurl Depth Slider in Header Strip */}
				<div className="flex items-center gap-2 bg-(--sunken) border border-(--line) px-2 py-0.5 rounded-[2px]">
					<Layers className="w-3 h-3 text-sky-400" />
					<Typography.Label size="xxs" tone="f4">
						Depth
					</Typography.Label>
					<span className="text-[10px] font-bold text-sky-300 tabular-nums">
						{unfurlDepth === 0
							? "0"
							: unfurlDepth >= maxGrammarDepth
								? `${unfurlDepth} (Max)`
								: `${unfurlDepth}`}
					</span>
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
						className="w-16 h-1 bg-(--line) rounded appearance-none cursor-pointer accent-sky-400"
					/>
					<Button
						variant="bare"
						size="xxs"
						onClick={() => {
							setUnfurlDepth(0);
							setToggledCapsules({});
						}}
						title="Collapse all"
						className="p-0.5 text-(--f4) hover:text-(--f1)"
					>
						<Minimize2 className="w-2.5 h-2.5" />
					</Button>
					<Button
						variant="bare"
						size="xxs"
						onClick={() => {
							setUnfurlDepth(Math.max(3, maxGrammarDepth));
							setToggledCapsules({});
						}}
						title="Expand all"
						className="p-0.5 text-(--f4) hover:text-(--f1)"
					>
						<Maximize2 className="w-2.5 h-2.5" />
					</Button>
				</div>
			</Section.Header>

			{/* Interactive Ribbon */}
			<div className="overflow-x-auto p-2 bg-(--sunken) min-h-12 flex items-center">
				<div className="inline-flex items-center gap-1 min-w-full">
					{nestedNodes.length === 0 ? (
						<span className="text-[10px] text-(--f4) font-mono">
							waiting for tokens…
						</span>
					) : (
						nestedNodes.map((node) => renderCapsuleNode(node, 0))
					)}
				</div>
			</div>
		</Section>
	);
};
