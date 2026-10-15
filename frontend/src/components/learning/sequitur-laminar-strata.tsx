import { Section } from "#/components/ui";
import { type GrammarSnapshot, RULE_PALETTE } from "./sequitur-engine";

interface SequiturLaminarStrataProps {
	snapshot: GrammarSnapshot;
	hoveredRuleId: string | null;
	onHoverRule: (ruleId: string | null) => void;
	hoveredCharIndex: number | null;
	onHoverChar: (charIndex: number | null) => void;
}

export const SequiturLaminarStrata = ({
	snapshot,
	hoveredRuleId,
	onHoverRule,
	hoveredCharIndex,
	onHoverChar,
}: SequiturLaminarStrataProps) => {
	const totalTokens = Math.max(
		1,
		snapshot.metrics.rawLength || snapshot.rootRule.symbols.length,
	);

	const sortedRulesByDepth = snapshot.rules
		.filter((r) => r.intervals.length > 0)
		.sort((a, b) => {
			if (b.depth !== a.depth) return b.depth - a.depth;
			return a.number - b.number;
		});

	return (
		<Section
			fit="pane"
			className="flex-1 min-h-0 bg-(--surface) flex flex-col font-mono"
		>
			<Section.Header
				size="s"
				title="Spatial Strata"
				meta={`Max Depth: ${snapshot.rootRule.depth} · ${sortedRulesByDepth.length} active tracks`}
			/>

			{/* Scrollable Tracks Area */}
			<div className="flex-1 overflow-auto p-2 bg-(--sunken)">
				<div
					className="min-w-[400px] flex flex-col gap-1"
					style={{ width: `${Math.max(100, totalTokens * 28 + 110)}px` }}
				>
					{/* Track 1: S Canopy */}
					<div className="grid grid-cols-[90px_1fr] items-center gap-1.5 py-0.5 border-b border-(--line)">
						<div className="flex flex-col pr-1">
							<span className="text-[10px] font-bold text-sky-400">
								S Canopy
							</span>
							<span className="text-[8.5px] text-(--f4)">top-level</span>
						</div>

						<div
							className="relative grid gap-0.5 h-6 items-center"
							style={{
								gridTemplateColumns: `repeat(${totalTokens}, minmax(0, 1fr))`,
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
									<button
										type="button"
										key={tok.id}
										onMouseEnter={() => {
											if (tok.ruleId) onHoverRule(tok.ruleId);
											onHoverChar(start);
										}}
										onMouseLeave={() => {
											onHoverRule(null);
											onHoverChar(null);
										}}
										onFocus={() => {
											if (tok.ruleId) onHoverRule(tok.ruleId);
											onHoverChar(start);
										}}
										onBlur={() => {
											onHoverRule(null);
											onHoverChar(null);
										}}
										className={`relative h-5 flex items-center justify-center rounded-[2px] px-0.5 text-[9.5px] font-semibold border transition-colors select-none ${
											palette
												? `${palette.bg} ${palette.border} ${palette.text}`
												: "bg-(--surface) border-(--line) text-(--f2)"
										} ${isHovered ? "ring-1 ring-sky-400 z-10" : ""}`}
										style={{
											gridColumn: `${start + 1} / ${end + 2}`,
										}}
										title={
											tok.ruleId
												? `${tok.ruleId} (“${ruleObj?.expansion}”, span ${start + 1}..${end + 1})`
												: `Terminal '${tok.value}' (pos ${start + 1})`
										}
									>
										<span className="truncate">{tok.value}</span>
									</button>
								);
							})}
						</div>
					</div>

					{/* Dedicated Track for each active rule */}
					{sortedRulesByDepth.map((rule) => {
						const palette = RULE_PALETTE[rule.colorIndex];
						const isHovered = hoveredRuleId === rule.id;

						return (
							<button
								type="button"
								key={rule.id}
								onMouseEnter={() => onHoverRule(rule.id)}
								onMouseLeave={() => onHoverRule(null)}
								onFocus={() => onHoverRule(rule.id)}
								onBlur={() => onHoverRule(null)}
								className={`w-full text-left grid grid-cols-[90px_1fr] items-center gap-1.5 py-0.5 rounded-[2px] transition-colors cursor-default ${
									isHovered ? "bg-(--raised)" : ""
								}`}
							>
								<div className="flex items-center justify-between pr-1">
									<span className={`text-[10px] font-bold ${palette.text}`}>
										{rule.id}
									</span>
									<span className="text-[8.5px] text-(--f4)">
										d{rule.depth}
									</span>
								</div>

								<div
									className="relative grid gap-0.5 h-5 items-center"
									style={{
										gridTemplateColumns: `repeat(${totalTokens}, minmax(0, 1fr))`,
									}}
								>
									{rule.intervals.map(([start, end]) => (
										<div
											key={`${rule.id}-span-${start}-${end}`}
											className={`h-5 flex items-center justify-center rounded-[2px] px-0.5 text-[9px] font-bold border transition-colors ${
												palette.bg
											} ${palette.border} ${palette.text} ${
												isHovered ? "ring-1 ring-sky-400 z-10" : ""
											}`}
											style={{
												gridColumn: `${start + 1} / ${end + 2}`,
											}}
											title={`${rule.id} (tokens ${start + 1}..${end + 1})`}
										>
											<span className="truncate">{rule.id}</span>
										</div>
									))}
								</div>
							</button>
						);
					})}
				</div>
			</div>
		</Section>
	);
};
