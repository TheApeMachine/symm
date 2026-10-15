import { useState } from "react";
import { Badge, Button, Chip, Flex, Section } from "#/components/ui";
import { type GrammarSnapshot, RULE_PALETTE } from "./sequitur-engine";

interface SequiturRulebookDeckProps {
	snapshot: GrammarSnapshot;
	hoveredRuleId: string | null;
	onHoverRule: (ruleId: string | null) => void;
	hoveredDigramKey: string | null;
	onHoverDigram: (key: string | null) => void;
}

export const SequiturRulebookDeck = ({
	snapshot,
	hoveredRuleId,
	onHoverRule,
}: SequiturRulebookDeckProps) => {
	const [selectedActionFilter, setSelectedActionFilter] =
		useState<string>("all");

	const filteredRules = snapshot.rules.filter((r) => {
		if (selectedActionFilter === "enter") return r.action === "enter";
		if (selectedActionFilter === "exit") return r.action === "exit";
		if (selectedActionFilter === "with-action") return Boolean(r.action);
		return true;
	});

	return (
		<Flex.Column className="h-full min-h-0 bg-(--surface) divide-y divide-(--line) font-mono">
			{/* Invariant Status Section */}
			<Section fit="content" className="shrink-0 bg-(--surface)">
				<Section.Header
					size="s"
					title="Grammar Invariants"
					meta="Nevill-Manning 1997"
				/>
				<div className="grid grid-cols-2 gap-2 p-2 bg-(--sunken)">
					{/* Invariant 1: Digram Uniqueness */}
					<div className="flex flex-col gap-0.5">
						<span className="text-[8.5px] uppercase text-(--f4)">
							1. Digram Uniqueness
						</span>
						<Badge
							size="xs"
							variant={
								snapshot.invariantStatus.digramUniqueness === "VIOLATED"
									? "error"
									: snapshot.invariantStatus.digramUniqueness === "REPAIRED"
										? "info"
										: "success"
							}
							label={
								snapshot.invariantStatus.digramUniqueness === "VIOLATED"
									? "Duplicate"
									: snapshot.invariantStatus.digramUniqueness === "REPAIRED"
										? "Fused"
										: "Nominal (≤1×)"
							}
						/>
					</div>

					{/* Invariant 2: Rule Utility */}
					<div className="flex flex-col gap-0.5">
						<span className="text-[8.5px] uppercase text-(--f4)">
							2. Rule Utility
						</span>
						<Badge
							size="xs"
							variant={
								snapshot.invariantStatus.ruleUtility === "VIOLATED"
									? "warning"
									: snapshot.invariantStatus.ruleUtility === "REPAIRED"
										? "info"
										: "success"
							}
							label={
								snapshot.invariantStatus.ruleUtility === "VIOLATED"
									? "Underused"
									: snapshot.invariantStatus.ruleUtility === "REPAIRED"
										? "Dissolved"
										: "Nominal (≥2×)"
							}
						/>
					</div>
				</div>
			</Section>

			{/* Active Rules List Section */}
			<Section
				fit="pane"
				className="flex-1 min-h-0 flex flex-col bg-(--surface)"
			>
				<Section.Header
					size="s"
					title="Production Rules"
					meta={`${snapshot.rules.length} active`}
				>
					{snapshot.rules.some((r) => r.action) && (
						<div className="flex items-center gap-1">
							{["all", "enter", "exit"].map((f) => (
								<Button
									key={f}
									size="xxs"
									variant={selectedActionFilter === f ? "solid" : "quiet"}
									tone={selectedActionFilter === f ? "info" : "muted"}
									onClick={() => setSelectedActionFilter(f)}
								>
									{f}
								</Button>
							))}
						</div>
					)}
				</Section.Header>

				<div className="flex-1 overflow-y-auto divide-y divide-(--line)">
					{filteredRules.length === 0 ? (
						<div className="p-3 text-[10px] text-(--f4) text-center">
							no active rules
						</div>
					) : (
						filteredRules.map((rule) => {
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
									className={`w-full text-left px-2.5 py-1.5 flex flex-col gap-0.5 transition-colors cursor-pointer ${
										isHovered ? "bg-(--raised)" : "hover:bg-(--sunken)"
									}`}
								>
									<div className="flex items-center justify-between">
										<div className="flex items-center gap-1.5">
											<span
												className={`px-1 py-0.2 rounded-[2px] text-[10px] font-bold border ${palette.border} ${palette.bg} ${palette.text}`}
											>
												{rule.id}
											</span>
											<span className="text-[11px] font-semibold text-(--f1)">
												→ {rule.symbols.map((s) => s.value).join(" ")}
											</span>
										</div>

										<div className="flex items-center gap-1.5">
											{rule.action && (
												<Badge
													size="xxs"
													variant={
														rule.action === "enter" ? "success" : "error"
													}
													label={rule.action}
												/>
											)}
											<Chip
												size="xs"
												variant="bare"
												label=""
												value={`${rule.refCount}×`}
											/>
										</div>
									</div>

									<div className="flex items-center justify-between text-[9px] text-(--f4)">
										<span className="truncate" title={rule.expansion}>
											path: {rule.expansion}
										</span>
										<span>depth {rule.depth}</span>
									</div>
								</button>
							);
						})
					)}
				</div>
			</Section>
		</Flex.Column>
	);
};
