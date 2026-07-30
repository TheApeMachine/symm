"use client";

import { useEffect, useMemo, useRef } from "react";
import { Badge } from "#/components/ui/badge";
import {
	Card,
	CardFrame,
	CardFrameDescription,
	CardFrameHeader,
	CardFrameTitle,
	CardPanel,
} from "#/components/ui/card";
import { Empty } from "#/components/ui/empty";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { getMockTensorStats, type TensorStats } from "#/lib/mock-tensor-stats";
import type { Graph } from "./core/graph";

const mergeData = (
	node: { data?: Array<Record<string, unknown>> } | undefined,
): Record<string, unknown> => {
	const merged: Record<string, unknown> = {};
	for (const entry of node?.data ?? []) {
		for (const [key, value] of Object.entries(entry)) {
			merged[key] = value;
		}
	}
	return merged;
};

const parseShape = (shape: unknown): number[] | null => {
	if (typeof shape !== "string" || shape.length === 0) return null;
	const dims = shape
		.split(/[×x*,\s]+/)
		.filter(Boolean)
		.map((part) => Number.parseInt(part, 10));
	return dims.every((dim) => Number.isFinite(dim) && dim > 0) ? dims : null;
};

const formatNumber = (value: number, fractionDigits = 4): string => {
	if (!Number.isFinite(value)) return "n/a";
	if (Math.abs(value) < 1e-3) return value.toExponential(2);
	return value.toFixed(fractionDigits);
};

const Histogram = ({ stats }: { stats: TensorStats }) => {
	const max = useMemo(
		() => stats.histogram.reduce((acc, bin) => Math.max(acc, bin.count), 1),
		[stats.histogram],
	);

	return (
		<Flex.Column gap={2}>
			<Typography.Span
				className="text-[10px] font-medium uppercase tracking-wider"
				variant="muted"
			>
				Weight distribution
			</Typography.Span>
			<Flex.Row className="h-32 items-end gap-px">
				{stats.histogram.map((bin) => (
					<div
						className="flex-1 rounded-t bg-info/60"
						key={bin.bin}
						style={{ height: `${(bin.count / max) * 100}%` }}
						title={`${bin.count}`}
					/>
				))}
			</Flex.Row>
			<Flex.Row className="justify-between font-mono text-[10px] text-muted-foreground">
				<span>{formatNumber(stats.min)}</span>
				<span>0</span>
				<span>{formatNumber(stats.max)}</span>
			</Flex.Row>
		</Flex.Column>
	);
};

const Heatmap = ({ stats }: { stats: TensorStats }) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;
		if (!canvas) return;

		const { rows, cols, values } = stats.heatmap;
		const ctx = canvas.getContext("2d");
		if (!ctx) return;

		const scale = 8;
		canvas.width = cols * scale;
		canvas.height = rows * scale;

		// Normalize to [-absMax, absMax] for a diverging map.
		let absMax = 0;
		for (let index = 0; index < values.length; index++) {
			const abs = Math.abs(values[index]);
			if (abs > absMax) absMax = abs;
		}

		const denom = Math.max(absMax, 1e-9);
		const image = ctx.createImageData(cols, rows);

		for (let index = 0; index < values.length; index++) {
			const value = values[index] / denom;
			const offset = index * 4;
			// Diverging blue (negative) → black (zero) → orange (positive).
			if (value >= 0) {
				image.data[offset] = Math.round(255 * value);
				image.data[offset + 1] = Math.round(150 * value);
				image.data[offset + 2] = 0;
			} else {
				const v = -value;
				image.data[offset] = 0;
				image.data[offset + 1] = Math.round(150 * v);
				image.data[offset + 2] = Math.round(255 * v);
			}
			image.data[offset + 3] = 255;
		}

		const tempCanvas = document.createElement("canvas");
		tempCanvas.width = cols;
		tempCanvas.height = rows;
		tempCanvas.getContext("2d")?.putImageData(image, 0, 0);

		ctx.imageSmoothingEnabled = false;
		ctx.clearRect(0, 0, canvas.width, canvas.height);
		ctx.drawImage(tempCanvas, 0, 0, canvas.width, canvas.height);
	}, [stats]);

	return (
		<Flex.Column gap={2}>
			<Typography.Span
				className="text-[10px] font-medium uppercase tracking-wider"
				variant="muted"
			>
				Slice preview ({stats.heatmap.rows}×{stats.heatmap.cols})
			</Typography.Span>
			<canvas
				className="w-full rounded-md border border-border bg-black/40"
				ref={canvasRef}
				style={{ imageRendering: "pixelated" }}
			/>
		</Flex.Column>
	);
};

const Stat = ({ label, value }: { label: string; value: React.ReactNode }) => {
	return (
		<Flex.Column gap={1}>
			<Typography.Span
				className="text-[10px] font-medium uppercase tracking-wider"
				variant="muted"
			>
				{label}
			</Typography.Span>
			<Typography.Span className="font-mono text-sm">{value}</Typography.Span>
		</Flex.Column>
	);
};

export const WeightsPanel = ({
	graph,
	selectedName,
}: {
	graph: Graph | undefined;
	selectedName: string | null;
}) => {
	if (!graph || !selectedName) {
		return (
			<Empty className="py-12">
				<Empty.Header>
					<Empty.Title className="text-sm">No tensor selected</Empty.Title>
					<Empty.Description>
						Pick a leaf node from the graph or the Node panel to see its weight
						statistics.
					</Empty.Description>
				</Empty.Header>
			</Empty>
		);
	}

	const node = graph.nodes[selectedName];
	const merged = mergeData(node);
	const shape = parseShape(merged.shape);
	const dtype = typeof merged.dtype === "string" ? merged.dtype : "F32";

	if (!shape) {
		return (
			<Empty className="py-12">
				<Empty.Header>
					<Empty.Title className="text-sm">Group node</Empty.Title>
					<Empty.Description>
						This node groups other tensors. Drill into one of its children to
						inspect actual weights.
					</Empty.Description>
				</Empty.Header>
			</Empty>
		);
	}

	const stats = getMockTensorStats(selectedName, dtype, shape);

	return (
		<Flex.Column className="gap-4 p-4">
			<Flex.Column gap={1}>
				<Typography.PageTitle className="truncate text-xl">
					{selectedName.split(".").at(-1)}
				</Typography.PageTitle>
				<Flex.Row className="flex-wrap items-center gap-2">
					<Badge size="sm" variant="outline">
						{dtype}
					</Badge>
					<Badge size="sm" variant="outline">
						{shape.join(" × ")}
					</Badge>
					<Typography.Span className="text-xs" variant="muted">
						mock data · awaiting backend tensor stream
					</Typography.Span>
				</Flex.Row>
			</Flex.Column>

			<CardFrame>
				<CardFrameHeader>
					<CardFrameTitle>Statistics</CardFrameTitle>
					<CardFrameDescription>
						Summary moments over the tensor's elements.
					</CardFrameDescription>
				</CardFrameHeader>
				<Card>
					<CardPanel>
						<Flex.Row className="flex-wrap gap-6">
							<Stat label="L2 norm" value={formatNumber(stats.l2Norm, 2)} />
							<Stat label="Mean" value={formatNumber(stats.mean)} />
							<Stat label="Std" value={formatNumber(stats.std)} />
							<Stat
								label="Sparsity"
								value={`${(stats.sparsity * 100).toFixed(1)}%`}
							/>
							{stats.effectiveRank !== null ? (
								<Stat label="Eff. rank" value={stats.effectiveRank} />
							) : null}
						</Flex.Row>
					</CardPanel>
				</Card>
			</CardFrame>

			<CardFrame>
				<CardFrameHeader>
					<CardFrameTitle>Distribution</CardFrameTitle>
					<CardFrameDescription>
						Histogram of element values.
					</CardFrameDescription>
				</CardFrameHeader>
				<Card>
					<CardPanel>
						<Histogram stats={stats} />
					</CardPanel>
				</Card>
			</CardFrame>

			{shape.length >= 2 ? (
				<CardFrame>
					<CardFrameHeader>
						<CardFrameTitle>Heatmap</CardFrameTitle>
						<CardFrameDescription>
							Diverging map (blue ↔ orange) over a top-left slice of the tensor.
							Sliced for performance.
						</CardFrameDescription>
					</CardFrameHeader>
					<Card>
						<CardPanel>
							<Heatmap stats={stats} />
						</CardPanel>
					</Card>
				</CardFrame>
			) : null}
		</Flex.Column>
	);
};
