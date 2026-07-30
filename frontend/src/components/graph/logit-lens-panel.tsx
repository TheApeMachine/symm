"use client";

import { PlayIcon, SparklesIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import {
	Card,
	CardFrame,
	CardFrameAction,
	CardFrameDescription,
	CardFrameHeader,
	CardFrameTitle,
	CardPanel,
} from "#/components/ui/card";
import { Field } from "#/components/ui/field";
import { Flex } from "#/components/ui/flex";
import { Input } from "#/components/ui/input";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

export type LogitLensCell = {
	token: string;
	probability: number;
	alternatives: ReadonlyArray<{ token: string; probability: number }>;
};

export type LogitLensRun = {
	prompt: string;
	tokens: ReadonlyArray<string>;
	layers: ReadonlyArray<{ index: number; label: string }>;
	cells: ReadonlyArray<ReadonlyArray<LogitLensCell>>;
};

/*
mockRun fabricates a plausible logit-lens trace for a prompt. Used to
exercise the UI before the backend inference pipeline lands. Early
layers converge on noisy tokens, later layers progressively lock onto
the input tokens themselves.
*/
const MOCK_VOCAB = [
	"the",
	"of",
	"and",
	"to",
	"a",
	"in",
	"is",
	"it",
	"you",
	"that",
	"he",
	"was",
	"for",
	"on",
	"are",
	"with",
	"as",
	"his",
	"they",
	"at",
];

const mockRun = (prompt: string, layerCount: number): LogitLensRun => {
	const tokens =
		prompt.trim().length === 0
			? ["[empty]"]
			: prompt.trim().split(/\s+/).slice(0, 24);

	const layers = Array.from({ length: layerCount }, (_, index) => ({
		index,
		label: `L${index}`,
	}));

	const cells = layers.map((layer) =>
		tokens.map((target, position) => {
			const depth = layer.index / Math.max(1, layerCount - 1);
			const lockedIn = Math.random() < depth ** 1.6;
			const winner = lockedIn
				? target
				: MOCK_VOCAB[(position + layer.index) % MOCK_VOCAB.length];
			const probability =
				0.1 + depth * 0.7 + (lockedIn ? 0.15 : 0) - Math.random() * 0.15;
			const clamped = Math.max(0.02, Math.min(0.99, probability));

			const alternatives = Array.from({ length: 3 }, (_, alternativeIndex) => ({
				token:
					MOCK_VOCAB[
						(position + alternativeIndex + layer.index + 1) % MOCK_VOCAB.length
					],
				probability: clamped * 0.35 * (1 - alternativeIndex * 0.28),
			}));

			return { token: winner, probability: clamped, alternatives };
		}),
	);

	return { prompt, tokens, layers, cells };
};

const probabilityColor = (probability: number): string => {
	if (probability >= 0.8) return "bg-success/30";
	if (probability >= 0.5) return "bg-info/30";
	if (probability >= 0.25) return "bg-warning/30";
	return "bg-muted/40";
};

const ProbabilityCell = ({
	cell,
	highlight,
	matchTarget,
}: {
	cell: LogitLensCell;
	highlight: boolean;
	matchTarget: string | null;
}) => {
	const matchesTarget = matchTarget !== null && cell.token === matchTarget;

	return (
		<div
			className={cn(
				"relative flex h-9 min-w-0 flex-col justify-center rounded border border-border/60 px-1.5",
				probabilityColor(cell.probability),
				highlight && "ring-2 ring-primary",
				matchesTarget && "border-success/60",
			)}
			title={`${cell.token}: ${(cell.probability * 100).toFixed(1)}%`}
		>
			<Typography.Span className="truncate font-mono text-[10px] leading-none">
				{cell.token}
			</Typography.Span>
			<Typography.Span
				className="font-mono text-[9px] leading-none opacity-70"
				variant="muted"
			>
				{(cell.probability * 100).toFixed(0)}%
			</Typography.Span>
		</div>
	);
};

const LensGrid = ({
	run,
	selected,
	onSelectCell,
}: {
	run: LogitLensRun;
	selected: { layer: number; position: number } | null;
	onSelectCell: (cell: { layer: number; position: number }) => void;
}) => {
	return (
		<Flex.Column className="gap-2">
			<Flex.Row className="gap-1 pl-12">
				{run.tokens.map((token, position) => (
					<div
						className="flex-1 truncate text-center font-mono text-[10px] text-muted-foreground"
						// biome-ignore lint/suspicious/noArrayIndexKey: token position is the key
						key={position}
						title={token}
					>
						{token}
					</div>
				))}
			</Flex.Row>
			{[...run.cells].reverse().map((row, reversedIndex) => {
				const layerIndex = run.layers.length - 1 - reversedIndex;
				const layer = run.layers[layerIndex];

				return (
					<Flex.Row className="items-center gap-1" key={layer.index}>
						<div className="w-10 shrink-0 text-right font-mono text-[10px] text-muted-foreground">
							{layer.label}
						</div>
						<Flex.Row className="flex-1 gap-1">
							{row.map((cell, position) => (
								<button
									className="flex-1 min-w-0 cursor-pointer"
									// biome-ignore lint/suspicious/noArrayIndexKey: position is the key
									key={position}
									onClick={() => onSelectCell({ layer: layer.index, position })}
									type="button"
								>
									<ProbabilityCell
										cell={cell}
										highlight={
											selected?.layer === layer.index &&
											selected?.position === position
										}
										matchTarget={run.tokens[position] ?? null}
									/>
								</button>
							))}
						</Flex.Row>
					</Flex.Row>
				);
			})}
		</Flex.Column>
	);
};

const CellDetails = ({
	run,
	selected,
}: {
	run: LogitLensRun;
	selected: { layer: number; position: number };
}) => {
	const cell = run.cells[selected.layer]?.[selected.position];
	const target = run.tokens[selected.position];

	if (!cell) return null;

	const all = [
		{ token: cell.token, probability: cell.probability },
		...cell.alternatives,
	].sort((a, b) => b.probability - a.probability);

	return (
		<Flex.Column className="gap-3">
			<Flex.Column className="gap-1">
				<Typography.Span
					className="text-[10px] font-medium uppercase tracking-wider"
					variant="muted"
				>
					Position {selected.position} · Layer {selected.layer}
				</Typography.Span>
				<Typography.Span className="font-mono text-sm">
					target: <span className="font-semibold">{target}</span>
				</Typography.Span>
			</Flex.Column>
			<Flex.Column className="gap-1.5">
				{all.map((alt, index) => (
					<Flex.Column
						className="gap-0.5"
						// biome-ignore lint/suspicious/noArrayIndexKey: ordered rank is the key
						key={index}
					>
						<Flex.Row className="items-center justify-between">
							<Typography.Span className="font-mono text-xs">
								{alt.token}
							</Typography.Span>
							<Typography.Span
								className="font-mono text-[11px]"
								variant="muted"
							>
								{(alt.probability * 100).toFixed(1)}%
							</Typography.Span>
						</Flex.Row>
						<div className="h-1 overflow-hidden rounded-full bg-border/60">
							<div
								className={cn(
									"h-full rounded-full",
									probabilityColor(alt.probability).replace("/30", ""),
								)}
								style={{ width: `${alt.probability * 100}%` }}
							/>
						</div>
					</Flex.Column>
				))}
			</Flex.Column>
		</Flex.Column>
	);
};

export const LogitLensPanel = ({ layerCount }: { layerCount: number }) => {
	const [prompt, setPrompt] = useState("The capital of France is");
	const [run, setRun] = useState<LogitLensRun | null>(null);
	const [selected, setSelected] = useState<{
		layer: number;
		position: number;
	} | null>(null);

	const effectiveLayerCount = useMemo(
		() => Math.max(4, Math.min(48, layerCount)),
		[layerCount],
	);

	const runMock = () => {
		const next = mockRun(prompt, effectiveLayerCount);
		setRun(next);
		setSelected({ layer: next.layers.length - 1, position: 0 });
	};

	return (
		<Flex.Column className="gap-4 p-4" fullHeight>
			<Flex.Column gap={1}>
				<Typography.PageTitle className="text-xl">
					Logit Lens
				</Typography.PageTitle>
				<Typography.Paragraph className="max-w-prose text-sm" variant="muted">
					Decode every layer's hidden state through the unembedding matrix and
					watch the model commit to a token. Backend wiring is pending — for now
					this runs against a mock so the UI shape is real.
				</Typography.Paragraph>
			</Flex.Column>

			<Field>
				<Field.Label htmlFor="logit-lens-prompt">Prompt</Field.Label>
				<Flex.Row className="gap-2">
					<Input
						id="logit-lens-prompt"
						onChange={(event) => setPrompt(event.target.value)}
						onKeyDown={(event) => {
							if (event.key === "Enter") runMock();
						}}
						placeholder="Enter a prompt to trace…"
						value={prompt}
					/>
					<Button onClick={runMock} type="button">
						<PlayIcon />
						Run
					</Button>
				</Flex.Row>
			</Field>

			{!run ? (
				<Flex.Column className="gap-2 rounded-xl border border-border border-dashed bg-card/30 p-6 text-center">
					<SparklesIcon
						aria-hidden
						className="size-6 self-center text-muted-foreground"
					/>
					<Typography.Span className="text-sm">
						Hit Run to see the lens.
					</Typography.Span>
					<Typography.Span className="text-xs" variant="muted">
						{effectiveLayerCount} layers will be sampled.
					</Typography.Span>
				</Flex.Column>
			) : (
				<Flex.Column className="gap-3">
					<CardFrame>
						<CardFrameHeader>
							<CardFrameTitle>Lens trace</CardFrameTitle>
							<CardFrameDescription>
								Top-1 predicted token per (layer × position). Greener = higher
								confidence.
							</CardFrameDescription>
							<CardFrameAction>
								<Badge size="sm" variant="outline">
									{run.layers.length} × {run.tokens.length}
								</Badge>
							</CardFrameAction>
						</CardFrameHeader>
						<Card>
							<CardPanel className="overflow-x-auto p-3">
								<LensGrid
									onSelectCell={setSelected}
									run={run}
									selected={selected}
								/>
							</CardPanel>
						</Card>
					</CardFrame>

					{selected ? (
						<CardFrame>
							<CardFrameHeader>
								<CardFrameTitle>Top alternatives</CardFrameTitle>
								<CardFrameDescription>
									Distribution at the highlighted cell.
								</CardFrameDescription>
							</CardFrameHeader>
							<Card>
								<CardPanel>
									<CellDetails run={run} selected={selected} />
								</CardPanel>
							</Card>
						</CardFrame>
					) : null}
				</Flex.Column>
			)}
		</Flex.Column>
	);
};
