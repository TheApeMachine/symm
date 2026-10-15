import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { focusAtom, signals } from "#/collections/app";
import { Badge, Chip, Flex, Grid, Toolbar, Typography } from "#/components/ui";
import { hubBaseUrl } from "#/lib/hub";
import {
	type GrammarSnapshot,
	grammarResponseToSnapshot,
} from "./sequitur-engine";
import { SequiturLaminarStrata } from "./sequitur-laminar-strata";
import { SequiturRulebookDeck } from "./sequitur-rulebook-deck";
import { SequiturTelescopingTape } from "./sequitur-telescoping-tape";

export const SequiturGrammarViz = () => {
	const focus = useSelector(focusAtom, (s) => s);

	// Synchronized hover states across lenses
	const [hoveredRuleId, setHoveredRuleId] = useState<string | null>(null);
	const [hoveredCharIndex, setHoveredCharIndex] = useState<number | null>(null);
	const [hoveredDigramKey, setHoveredDigramKey] = useState<string | null>(null);

	// Live Grammar Snapshot & Error
	const [liveSnapshot, setLiveSnapshot] = useState<GrammarSnapshot | null>(
		null,
	);
	const [liveError, setLiveError] = useState<string | null>(null);

	// Fetch baseline grammar from REST endpoint
	useEffect(() => {
		let alive = true;
		const fetchGrammar = async () => {
			try {
				const res = await fetch(
					`${hubBaseUrl()}/cognition/grammar?symbol=${encodeURIComponent(focus)}`,
				);
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				const data = await res.json();
				if (alive && data && Array.isArray(data.rules)) {
					setLiveSnapshot((prev) =>
						grammarResponseToSnapshot(data, {
							surprise: prev?.metrics?.surprise,
							activeTokens: prev?.metrics?.activeTokens,
						}),
					);
					setLiveError(null);
				}
			} catch (err: unknown) {
				if (alive) {
					setLiveError(
						err instanceof Error ? err.message : "Failed to load grammar",
					);
				}
			}
		};

		fetchGrammar();
		const interval = window.setInterval(fetchGrammar, 5000);
		return () => {
			alive = false;
			window.clearInterval(interval);
		};
	}, [focus]);

	// Listen for live measurements from training stream
	const trainingMeasurements = useSelector(signals.training, (state) => {
		const ring = state[focus];
		if (!ring) return null;
		const all = ring.toArray();
		return all.length > 0 ? all[all.length - 1] : null;
	});

	useEffect(() => {
		if (!trainingMeasurements?.provenance) return;

		let grammarJson = "";
		let surpriseVal: number | undefined;
		let activeTokensVal: string | undefined;

		for (const p of trainingMeasurements.provenance) {
			if (p?.name === "sequitur_grammar" && p.value) {
				grammarJson = String(p.value);
			} else if (p?.name === "resonance_surprise" && p.value) {
				const num = Number.parseFloat(String(p.value));
				if (!Number.isNaN(num)) {
					surpriseVal = num;
				}
			} else if (p?.name === "active_tokens" && p.value) {
				activeTokensVal = String(p.value);
			}
		}

		if (grammarJson) {
			try {
				const parsed = JSON.parse(grammarJson);
				setLiveSnapshot((prev) =>
					grammarResponseToSnapshot(parsed, {
						surprise: surpriseVal ?? prev?.metrics?.surprise,
						activeTokens: activeTokensVal ?? prev?.metrics?.activeTokens,
					}),
				);
				setLiveError(null);
			} catch {
				// parse error ignored
			}
		} else if (surpriseVal !== undefined || activeTokensVal !== undefined) {
			setLiveSnapshot((prev) => {
				if (!prev) return null;
				return {
					...prev,
					metrics: {
						...prev.metrics,
						surprise: surpriseVal ?? prev.metrics.surprise,
						activeTokens: activeTokensVal ?? prev.metrics.activeTokens,
					},
				};
			});
		}
	}, [trainingMeasurements]);

	return (
		<Flex.Column
			fullWidth
			className="h-full min-h-0 bg-(--bg) text-(--f2) font-mono"
		>
			{/* Top Terminal Toolbar: Title + Symbol Badge + Live Status + Metrics */}
			<Toolbar
				size="s"
				className="border-b border-(--line) bg-(--surface) px-3 gap-3"
			>
				<Flex.Row align="center" gap={2}>
					<Typography.Label
						size="s"
						tone="f1"
						className="font-bold tracking-wider"
					>
						Sequitur Grammar
					</Typography.Label>
					<Badge size="xs" variant="warning" label={focus} />
					<div className="flex items-center gap-1.5 px-1.5 py-0.5 rounded-[2px] bg-(--sunken) border border-(--line) text-[10px] text-emerald-400">
						<span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
						<span className="font-bold tracking-wide">LIVE</span>
					</div>
				</Flex.Row>

				<Toolbar.Spacer />

				{/* Compact Stat Chips */}
				<Flex.Row align="center" gap={2}>
					<Chip
						size="xs"
						label="Surprise"
						value={
							liveSnapshot?.metrics?.surprise !== undefined
								? liveSnapshot.metrics.surprise.toFixed(4)
								: "0.0000"
						}
					/>
					<Chip
						size="xs"
						label="Chord"
						value={liveSnapshot?.metrics?.activeTokens || "—"}
					/>
					<Chip
						size="xs"
						label="Rules"
						value={String(liveSnapshot?.rules?.length ?? 0)}
					/>
					<Chip
						size="xs"
						label="Ratio"
						value={
							liveSnapshot
								? `${(liveSnapshot.metrics.compressionRatio * 100).toFixed(0)}%`
								: "—"
						}
					/>
				</Flex.Row>
			</Toolbar>

			{/* Main Grid: Flush Left Tape/Strata & Right Rulebook */}
			<Grid
				responsive={false}
				fullWidth
				className="flex-1 min-h-0 min-w-0 grid-cols-[minmax(0,1fr)_340px]"
			>
				{/* Left Pane: Telescoping Tape + Spatial Strata */}
				<Flex.Column className="min-h-0 border-r border-(--line) bg-(--sunken) overflow-hidden">
					{liveSnapshot ? (
						<>
							<SequiturTelescopingTape
								snapshot={liveSnapshot}
								hoveredRuleId={hoveredRuleId}
								onHoverRule={setHoveredRuleId}
								hoveredCharIndex={hoveredCharIndex}
								onHoverChar={setHoveredCharIndex}
								hoveredDigramKey={hoveredDigramKey}
								onHoverDigram={setHoveredDigramKey}
							/>

							<SequiturLaminarStrata
								snapshot={liveSnapshot}
								hoveredRuleId={hoveredRuleId}
								onHoverRule={setHoveredRuleId}
								hoveredCharIndex={hoveredCharIndex}
								onHoverChar={setHoveredCharIndex}
							/>
						</>
					) : (
						<Flex.Center className="flex-1 text-(--f4) text-xs">
							{liveError ? `Error: ${liveError}` : "awaiting grammar feed…"}
						</Flex.Center>
					)}
				</Flex.Column>

				{/* Right Pane: Invariants & Production Rulebook */}
				<Flex.Column className="min-h-0 bg-(--surface) overflow-hidden">
					{liveSnapshot ? (
						<SequiturRulebookDeck
							snapshot={liveSnapshot}
							hoveredRuleId={hoveredRuleId}
							onHoverRule={setHoveredRuleId}
							hoveredDigramKey={hoveredDigramKey}
							onHoverDigram={setHoveredDigramKey}
						/>
					) : (
						<Flex.Center className="flex-1 text-(--f4) text-xs">
							awaiting production rules…
						</Flex.Center>
					)}
				</Flex.Column>
			</Grid>
		</Flex.Column>
	);
};
