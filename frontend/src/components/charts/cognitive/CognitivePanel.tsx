import { useStore } from "@tanstack/react-store";
import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";
import { Badge } from "#/components/ui/badge";
import { Card, CardFrame, CardPanel } from "#/components/ui/card";
import { Flex } from "#/components/ui/flex";
import { cn } from "@/lib/utils";

const clampPercent = (value: number): number =>
	Math.min(100, Math.max(0, value * 100));

const MetricBar = ({
	label,
	value,
	max,
	tone = "sky",
}: {
	label: string;
	value: number;
	max: number;
	tone?: "sky" | "amber" | "rose" | "emerald";
}) => {
	const ratio = max > 0 ? value / max : 0;
	const width = clampPercent(ratio);

	return (
		<Flex.Column gap={1} fullWidth>
			<Flex.Row className="justify-between text-xs text-muted-foreground">
				<span>{label}</span>
				<span>
					{value.toFixed(3)} / {max.toFixed(3)}
				</span>
			</Flex.Row>
			<div className="h-2 w-full overflow-hidden rounded-full bg-muted/40">
				<div
					className={cn(
						"h-full rounded-full transition-all duration-300",
						tone === "sky" && "bg-sky-400",
						tone === "amber" && "bg-amber-400",
						tone === "rose" && "bg-rose-400",
						tone === "emerald" && "bg-emerald-400",
					)}
					style={{ width: `${width}%` }}
				/>
			</div>
		</Flex.Column>
	);
};

const ScopeCard = ({
	reading,
	active,
	onSelect,
}: {
	reading: CognitiveReading;
	active: boolean;
	onSelect: () => void;
}) => (
	<button
		type="button"
		onClick={onSelect}
		className={cn(
			"rounded-lg border px-3 py-2 text-left transition-colors",
			active ? "border-sky-400 bg-sky-400/10" : "border-border bg-card/40",
		)}
	>
		<Flex.Column gap={1}>
			<Flex.Row className="items-center justify-between gap-2">
				<span className="font-medium text-sm">{reading.scope}</span>
				{reading.sideline ? <Badge variant="outline">sideline</Badge> : null}
			</Flex.Row>
			<span className="text-muted-foreground text-xs">
				{reading.regimePrefix || "—"} · {reading.winnerClass || "pending"}
			</span>
		</Flex.Column>
	</button>
);

const ReadingDetail = ({ reading }: { reading: CognitiveReading }) => {
	const entropyRatio =
		reading.entropyThreshold > 0
			? reading.entropyBits / reading.entropyThreshold
			: 0;

	return (
		<Flex.Column
			gap={4}
			fullWidth
			fullHeight
			className="min-h-0 overflow-auto p-4"
		>
			<Flex.Row className="flex-wrap items-center gap-2">
				<Badge>{reading.regimePrefix || "no-regime"}</Badge>
				<Badge variant="secondary">cohort {reading.regimeCohort}</Badge>
				{reading.ambiguous ? <Badge variant="outline">ambiguous</Badge> : null}
				{reading.sideline ? (
					<Badge variant="destructive">sideline</Badge>
				) : null}
			</Flex.Row>

			<Card className="border-border/60 bg-card/30">
				<CardPanel className="gap-4 p-4">
					<p className="font-medium text-sm">DMT sequence</p>
					<p className="font-mono text-muted-foreground text-xs break-all">
						{reading.sequence || "—"}
					</p>
				</CardPanel>
			</Card>

			<Flex.Column gap={3}>
				<p className="font-medium text-sm">Entropy gate</p>
				<MetricBar
					label="Entropy bits vs threshold"
					value={reading.entropyBits}
					max={Math.max(reading.entropyThreshold, reading.entropyBits, 0.001)}
					tone={entropyRatio >= 1 ? "rose" : "emerald"}
				/>
				<MetricBar
					label="Class confidence"
					value={reading.classConfidence}
					max={1}
					tone="sky"
				/>
				<MetricBar
					label="Contrast evidence"
					value={reading.contrastEvidence}
					max={1}
					tone="amber"
				/>
			</Flex.Column>

			<Flex.Column gap={3}>
				<p className="font-medium text-sm">Lookahead beam</p>
				<MetricBar
					label="Beam score"
					value={reading.lookaheadScore}
					max={Math.max(
						reading.lookaheadScore,
						reading.prewarmScore ?? 0,
						0.001,
					)}
					tone="sky"
				/>
				<Flex.Row className="flex-wrap gap-2 text-xs text-muted-foreground">
					<span>paths: {reading.lookaheadPaths}</span>
					<span>winner: {reading.winnerClass || "—"}</span>
					{reading.prewarmPaths !== null ? (
						<span>prewarm paths: {reading.prewarmPaths}</span>
					) : null}
					{reading.prewarmScore !== null ? (
						<span>prewarm score: {reading.prewarmScore.toFixed(3)}</span>
					) : null}
				</Flex.Row>
			</Flex.Column>

			<Card className="border-border/60 bg-card/30">
				<CardPanel className="gap-2 p-4">
					<p className="font-medium text-sm">Beam tree (path count × score)</p>
					<div className="grid grid-cols-2 gap-2 md:grid-cols-4">
						{Array.from({ length: Math.max(reading.lookaheadPaths, 1) }).map(
							(_, index) => {
								const weight =
									reading.lookaheadPaths > 0
										? reading.lookaheadScore /
											Math.max(reading.lookaheadPaths - index, 1)
										: 0;

								return (
									<div
										key={`${reading.scope}-path-${index}`}
										className="rounded-md border border-border/60 bg-background/40 p-2"
									>
										<p className="text-muted-foreground text-xs">
											branch {index + 1}
										</p>
										<p className="font-mono text-sm">{weight.toFixed(3)}</p>
									</div>
								);
							},
						)}
					</div>
				</CardPanel>
			</Card>
		</Flex.Column>
	);
};

export const CognitivePanel = () => {
	const { readings, selectedScope } = useStore(cognitiveStore);
	const { selectScope } = cognitiveStore.actions;
	const scopes = cognitiveScopes();
	const activeScope =
		selectedScope !== "" && readings[selectedScope]
			? selectedScope
			: (scopes[0] ?? "");
	const activeReading = activeScope ? readings[activeScope] : null;

	return (
		<CardFrame className="h-full w-full">
			<Card className="h-full w-full overflow-hidden">
				<CardPanel className="grid h-full min-h-0 grid-cols-1 gap-0 p-0 lg:grid-cols-[220px_1fr]">
					<Flex.Column
						gap={2}
						className="min-h-0 overflow-auto border-border/60 border-b p-3 lg:border-r lg:border-b-0"
					>
						<p className="font-medium text-sm">Scopes</p>
						{scopes.length === 0 ? (
							<p className="text-muted-foreground text-xs">
								Waiting for cognitive frames from the backend…
							</p>
						) : (
							scopes.map((scope) => (
								<ScopeCard
									key={scope}
									reading={readings[scope]}
									active={scope === activeScope}
									onSelect={() => selectScope(scope)}
								/>
							))
						)}
					</Flex.Column>
					{activeReading ? (
						<ReadingDetail reading={activeReading} />
					) : (
						<Flex.Column
							className="items-center justify-center p-6 text-muted-foreground text-sm"
							fullHeight
							fullWidth
						>
							Cognitive memory has not sealed a reading yet.
						</Flex.Column>
					)}
				</CardPanel>
			</Card>
		</CardFrame>
	);
};
