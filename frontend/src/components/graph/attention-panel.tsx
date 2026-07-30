"use client";

import { PlayIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
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
import {
	type AttentionPattern,
	getMockAttentionPattern,
} from "#/lib/mock-tensor-stats";
import { cn } from "#/lib/utils";

const HeadMatrix = ({
	pattern,
	layer,
	head,
	highlighted,
	onSelect,
}: {
	pattern: AttentionPattern;
	layer: number;
	head: number;
	highlighted: boolean;
	onSelect: () => void;
}) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;
		if (!canvas) return;

		const ctx = canvas.getContext("2d");
		if (!ctx) return;

		const { tokens, matrix, heads } = pattern;
		const tokenCount = tokens.length;
		const stride = tokenCount * tokenCount;
		const offset = (layer * heads + head) * stride;
		const scale = 6;

		canvas.width = tokenCount * scale;
		canvas.height = tokenCount * scale;

		const image = ctx.createImageData(tokenCount, tokenCount);
		for (let i = 0; i < tokenCount; i++) {
			for (let j = 0; j < tokenCount; j++) {
				const value = matrix[offset + i * tokenCount + j];
				const pixel = (i * tokenCount + j) * 4;
				const intensity = Math.round(255 * Math.min(1, value * 1.5));
				image.data[pixel] = Math.round(intensity * 0.95);
				image.data[pixel + 1] = Math.round(intensity * 0.55);
				image.data[pixel + 2] = Math.round(intensity * 0.15);
				image.data[pixel + 3] = 255;
			}
		}

		const tmp = document.createElement("canvas");
		tmp.width = tokenCount;
		tmp.height = tokenCount;
		tmp.getContext("2d")?.putImageData(image, 0, 0);
		ctx.imageSmoothingEnabled = false;
		ctx.clearRect(0, 0, canvas.width, canvas.height);
		ctx.drawImage(tmp, 0, 0, canvas.width, canvas.height);
	}, [pattern, layer, head]);

	return (
		<button
			className={cn(
				"flex flex-col gap-1 rounded-md border p-1 transition-all",
				highlighted
					? "border-primary bg-primary/5"
					: "border-border bg-card/40 hover:border-ring/40",
			)}
			onClick={onSelect}
			type="button"
		>
			<Typography.Span className="px-1 font-mono text-[9px]" variant="muted">
				L{layer}·H{head}
			</Typography.Span>
			<canvas
				className="w-full rounded-sm bg-black"
				ref={canvasRef}
				style={{ imageRendering: "pixelated", aspectRatio: "1 / 1" }}
			/>
		</button>
	);
};

const TokensRibbon = ({ tokens }: { tokens: ReadonlyArray<string> }) => {
	return (
		<Flex.Row className="flex-wrap gap-1">
			{tokens.map((token, index) => (
				<span
					className="rounded bg-muted/60 px-1.5 py-0.5 font-mono text-[10px]"
					// biome-ignore lint/suspicious/noArrayIndexKey: position is the key
					key={index}
				>
					{token}
				</span>
			))}
		</Flex.Row>
	);
};

export const AttentionPanel = ({
	layerCount,
	headCount = 8,
}: {
	layerCount: number;
	headCount?: number;
}) => {
	const [prompt, setPrompt] = useState("The capital of France is");
	const [pattern, setPattern] = useState<AttentionPattern | null>(null);
	const [focused, setFocused] = useState<{
		layer: number;
		head: number;
	} | null>(null);

	const effectiveLayers = useMemo(
		() => Math.max(2, Math.min(24, layerCount)),
		[layerCount],
	);
	const effectiveHeads = useMemo(
		() => Math.max(2, Math.min(16, headCount)),
		[headCount],
	);

	const run = () => {
		const next = getMockAttentionPattern(
			prompt,
			effectiveLayers,
			effectiveHeads,
		);
		setPattern(next);
		setFocused({ layer: 0, head: 0 });
	};

	return (
		<Flex.Column className="gap-4 p-4">
			<Flex.Column gap={1}>
				<Typography.PageTitle className="text-xl">
					Attention
				</Typography.PageTitle>
				<Typography.Paragraph className="max-w-prose text-sm" variant="muted">
					Each cell is a heads-eye view of a layer's attention pattern. Click
					one to inspect token-token weights up close.
				</Typography.Paragraph>
			</Flex.Column>

			<Field>
				<Field.Label htmlFor="attention-prompt">Prompt</Field.Label>
				<Flex.Row className="gap-2">
					<Input
						id="attention-prompt"
						onChange={(event) => setPrompt(event.target.value)}
						onKeyDown={(event) => {
							if (event.key === "Enter") run();
						}}
						placeholder="Tokens to attend over…"
						value={prompt}
					/>
					<Button onClick={run} type="button">
						<PlayIcon />
						Run
					</Button>
				</Flex.Row>
			</Field>

			{pattern ? (
				<>
					<CardFrame>
						<CardFrameHeader>
							<CardFrameTitle>Heads grid</CardFrameTitle>
							<CardFrameDescription>
								Layers down, heads across.
							</CardFrameDescription>
							<CardFrameAction>
								<Badge size="sm" variant="outline">
									{pattern.layers}L × {pattern.heads}H
								</Badge>
							</CardFrameAction>
						</CardFrameHeader>
						<Card>
							<CardPanel className="p-3">
								<Flex.Column gap={2}>
									<TokensRibbon tokens={pattern.tokens} />
									<div
										className="grid gap-1"
										style={{
											gridTemplateColumns: `repeat(${pattern.heads}, minmax(0, 1fr))`,
										}}
									>
										{Array.from({ length: pattern.layers * pattern.heads }).map(
											(_, index) => {
												const layer = Math.floor(index / pattern.heads);
												const head = index % pattern.heads;
												return (
													<HeadMatrix
														head={head}
														highlighted={
															focused?.layer === layer && focused.head === head
														}
														key={`${layer}-${head}`}
														layer={layer}
														onSelect={() => setFocused({ layer, head })}
														pattern={pattern}
													/>
												);
											},
										)}
									</div>
								</Flex.Column>
							</CardPanel>
						</Card>
					</CardFrame>

					{focused ? (
						<CardFrame>
							<CardFrameHeader>
								<CardFrameTitle>
									Focus · L{focused.layer} · H{focused.head}
								</CardFrameTitle>
								<CardFrameDescription>
									Per-token attention weights for the selected head.
								</CardFrameDescription>
							</CardFrameHeader>
							<Card>
								<CardPanel>
									<FocusedHead pattern={pattern} focused={focused} />
								</CardPanel>
							</Card>
						</CardFrame>
					) : null}
				</>
			) : (
				<Flex.Column className="gap-2 rounded-xl border border-border border-dashed bg-card/30 p-6 text-center">
					<Typography.Span className="text-sm">
						Hit Run to draw {effectiveLayers}×{effectiveHeads} attention heads.
					</Typography.Span>
				</Flex.Column>
			)}
		</Flex.Column>
	);
};

const FocusedHead = ({
	pattern,
	focused,
}: {
	pattern: AttentionPattern;
	focused: { layer: number; head: number };
}) => {
	const { tokens, matrix, heads } = pattern;
	const tokenCount = tokens.length;
	const offset =
		(focused.layer * heads + focused.head) * tokenCount * tokenCount;

	return (
		<Flex.Column className="gap-2 overflow-x-auto">
			<Flex.Row className="gap-1 pl-16">
				{tokens.map((token, index) => (
					<div
						className="w-12 truncate text-center font-mono text-[10px] text-muted-foreground"
						// biome-ignore lint/suspicious/noArrayIndexKey: position is the key
						key={index}
						title={token}
					>
						{token}
					</div>
				))}
			</Flex.Row>
			{tokens.map((token, rowIndex) => (
				<Flex.Row className="items-center gap-1" key={`matrix-row-${token}`}>
					<div className="w-16 truncate text-right font-mono text-[10px] text-muted-foreground">
						{token}
					</div>
					{tokens.map((columnToken, columnIndex) => {
						const weight = matrix[offset + rowIndex * tokenCount + columnIndex];
						const intensity = Math.min(1, weight * 1.5);
						return (
							<div
								className="h-6 w-12 rounded-sm"
								key={`${token}-${columnToken}`}
								style={{
									backgroundColor: `rgba(255, 140, 30, ${intensity})`,
								}}
								title={`${(weight * 100).toFixed(1)}%`}
							/>
						);
					})}
				</Flex.Row>
			))}
		</Flex.Column>
	);
};
