"use client";

import { ChevronRightIcon, CopyIcon, LayersIcon } from "lucide-react";
import { useMemo } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
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
import { cn } from "#/lib/utils";
import type { Graph, Node as GraphNode } from "./core/graph";

/*
mergeNodeData walks a node's data array and produces a single flat
record. Later entries win on key collisions. Leaf tensor nodes have
just one entry; group nodes generally have none.
*/
const mergeNodeData = (node: GraphNode): Record<string, unknown> => {
	const merged: Record<string, unknown> = {};

	for (const entry of node.data ?? []) {
		for (const [key, value] of Object.entries(entry)) {
			merged[key] = value;
		}
	}

	return merged;
};

const parseShape = (shape: unknown): number[] | null => {
	if (typeof shape !== "string" || shape.length === 0) {
		return null;
	}

	const parts = shape.split(/[×x*,\s]+/).filter(Boolean);
	const dims = parts.map((part) => Number.parseInt(part, 10));

	if (dims.some((dim) => !Number.isFinite(dim) || dim <= 0)) {
		return null;
	}

	return dims;
};

const formatParams = (count: number): string => {
	if (count >= 1_000_000_000) {
		return `${(count / 1_000_000_000).toFixed(2)}B`;
	}

	if (count >= 1_000_000) {
		return `${(count / 1_000_000).toFixed(2)}M`;
	}

	if (count >= 1_000) {
		return `${(count / 1_000).toFixed(1)}K`;
	}

	return count.toLocaleString();
};

const childrenOf = (graph: Graph, parent: string): string[] => {
	const prefix = parent === "" ? "" : `${parent}.`;
	const out: string[] = [];

	for (const name of Object.keys(graph.nodes)) {
		if (!name.startsWith(prefix)) continue;
		const tail = name.slice(prefix.length);
		if (tail.length === 0) continue;
		if (tail.includes(".")) continue;
		out.push(name);
	}

	return out.sort();
};

const ancestorsOf = (name: string): string[] => {
	if (name === "__model__") return [];

	const parts = name.split(".");
	const out: string[] = ["__model__"];

	for (let index = 1; index < parts.length; index++) {
		out.push(parts.slice(0, index).join("."));
	}

	return out;
};

const Breadcrumbs = ({
	name,
	onNavigate,
}: {
	name: string;
	onNavigate: (name: string) => void;
}) => {
	const ancestors = ancestorsOf(name);

	if (ancestors.length === 0) return null;

	return (
		<Flex.Row className="flex-wrap items-center gap-1 text-xs">
			{ancestors.map((ancestor, index) => (
				<Flex.Row className="items-center gap-1" key={ancestor || "root"}>
					<button
						className="rounded px-1 py-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
						onClick={() => onNavigate(ancestor)}
						type="button"
					>
						{ancestor === "__model__" ? "model" : ancestor.split(".").at(-1)}
					</button>
					<ChevronRightIcon
						aria-hidden
						className="size-3 text-muted-foreground/60"
					/>
					{index === ancestors.length - 1 ? (
						<Typography.Span className="font-medium">
							{name.split(".").at(-1)}
						</Typography.Span>
					) : null}
				</Flex.Row>
			))}
		</Flex.Row>
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

const ShapeBreakdown = ({ dims }: { dims: number[] }) => {
	const total = dims.reduce((acc, dim) => acc * dim, 1);

	return (
		<Flex.Column gap={2}>
			<Typography.Span
				className="text-[10px] font-medium uppercase tracking-wider"
				variant="muted"
			>
				Shape breakdown
			</Typography.Span>
			<Flex.Row className="flex-wrap items-center gap-1">
				{dims.map((dim, index) => (
					<Flex.Row
						className="items-center gap-1"
						// biome-ignore lint/suspicious/noArrayIndexKey: shape dims are positional
						key={index}
					>
						<span className="rounded-md border border-border bg-muted/40 px-2 py-1 font-mono text-xs">
							{dim.toLocaleString()}
						</span>
						{index < dims.length - 1 ? (
							<Typography.Span variant="muted">×</Typography.Span>
						) : null}
					</Flex.Row>
				))}
				<Typography.Span className="ml-2 text-muted-foreground text-xs">
					= {formatParams(total)} elements
				</Typography.Span>
			</Flex.Row>
		</Flex.Column>
	);
};

const ChildrenList = ({
	graph,
	parent,
	onNavigate,
	onDrillInto,
}: {
	graph: Graph;
	parent: string;
	onNavigate: (name: string) => void;
	onDrillInto?: (name: string) => void;
}) => {
	const children = useMemo(() => childrenOf(graph, parent), [graph, parent]);

	if (children.length === 0) {
		return (
			<Empty className="py-6">
				<Empty.Header>
					<Empty.Title className="text-sm">Leaf node</Empty.Title>
					<Empty.Description>
						No further structure beneath this node.
					</Empty.Description>
				</Empty.Header>
			</Empty>
		);
	}

	return (
		<Flex.Column gap={1}>
			<Typography.Span
				className="text-[10px] font-medium uppercase tracking-wider"
				variant="muted"
			>
				Children ({children.length})
			</Typography.Span>
			{children.map((childName) => {
				const child = graph.nodes[childName];
				const merged = child ? mergeNodeData(child) : {};
				const dims = parseShape(merged.shape);
				const dtype = typeof merged.dtype === "string" ? merged.dtype : null;
				const subChildren = childrenOf(graph, childName);
				const tail = childName.slice(
					parent.length === 0 ? 0 : parent.length + 1,
				);

				const isGroupChild = subChildren.length > 0;

				return (
					<button
						className={cn(
							"flex items-center justify-between gap-2 rounded-lg border border-border bg-card/60 px-3 py-2 text-left transition-colors hover:border-primary/40 hover:bg-card",
						)}
						key={childName}
						onClick={() => {
							if (isGroupChild && onDrillInto) {
								onDrillInto(childName);
							} else {
								onNavigate(childName);
							}
						}}
						type="button"
					>
						<Flex.Column className="min-w-0 flex-1 gap-0.5">
							<Typography.Span className="truncate font-mono text-xs">
								{tail}
							</Typography.Span>
							{dims ? (
								<Typography.Span
									className="truncate text-[11px]"
									variant="muted"
								>
									{dims.join(" × ")}
									{dtype ? ` · ${dtype}` : ""}
								</Typography.Span>
							) : subChildren.length > 0 ? (
								<Typography.Span
									className="truncate text-[11px]"
									variant="muted"
								>
									{subChildren.length} children
								</Typography.Span>
							) : null}
						</Flex.Column>
						<ChevronRightIcon
							aria-hidden
							className="size-3.5 shrink-0 text-muted-foreground"
						/>
					</button>
				);
			})}
		</Flex.Column>
	);
};

export const NodeInspectorPanel = ({
	graph,
	selectedName,
	onSelect,
	onDrillInto,
}: {
	graph: Graph | undefined;
	selectedName: string | null;
	onSelect: (name: string | null) => void;
	onDrillInto?: (name: string) => void;
}) => {
	if (!graph) {
		return (
			<Empty className="py-12">
				<Empty.Header>
					<Empty.Title className="text-sm">No model loaded</Empty.Title>
					<Empty.Description>
						Pick a model from the dropdown to start inspecting.
					</Empty.Description>
				</Empty.Header>
			</Empty>
		);
	}

	if (!selectedName) {
		return (
			<Empty className="py-12">
				<Empty.Header>
					<Empty.Media variant="icon">
						<LayersIcon />
					</Empty.Media>
					<Empty.Title className="text-sm">Select a node</Empty.Title>
					<Empty.Description>
						Click any node in the graph to inspect its shape, dtype, and
						structure.
					</Empty.Description>
				</Empty.Header>
			</Empty>
		);
	}

	const node = graph.nodes[selectedName];

	if (!node) {
		return (
			<Empty className="py-12">
				<Empty.Header>
					<Empty.Title className="text-sm">Node not in graph</Empty.Title>
					<Empty.Description>
						"{selectedName}" isn't part of the current graph.
					</Empty.Description>
				</Empty.Header>
			</Empty>
		);
	}

	const merged = mergeNodeData(node);
	const dims = parseShape(merged.shape);
	const dtype = typeof merged.dtype === "string" ? merged.dtype : null;
	const tail = selectedName.split(".").at(-1) ?? selectedName;
	const params = dims?.reduce((acc, dim) => acc * dim, 1) ?? null;
	const isLeaf = childrenOf(graph, selectedName).length === 0;

	return (
		<Flex.Column className="gap-4 p-4" fullHeight>
			<Breadcrumbs name={selectedName} onNavigate={onSelect} />

			<Flex.Row className="items-start justify-between gap-2">
				<Flex.Column className="min-w-0 gap-1">
					<Typography.PageTitle className="truncate text-xl">
						{tail}
					</Typography.PageTitle>
					<Typography.Span
						className="break-all font-mono text-xs"
						variant="muted"
					>
						{selectedName}
					</Typography.Span>
				</Flex.Column>
				<Button
					aria-label="Copy node name"
					onClick={() => navigator.clipboard?.writeText(selectedName)}
					size="icon-sm"
					variant="ghost"
				>
					<CopyIcon />
				</Button>
			</Flex.Row>

			<Flex.Row className="flex-wrap items-center gap-2">
				<Badge size="sm" variant={isLeaf ? "outline" : "info"}>
					{isLeaf ? "Tensor" : "Group"}
				</Badge>
				{dtype ? (
					<Badge size="sm" variant="outline">
						{dtype}
					</Badge>
				) : null}
				{params !== null ? (
					<Badge size="sm" variant="outline">
						{formatParams(params)} params
					</Badge>
				) : null}
			</Flex.Row>

			<CardFrame>
				<CardFrameHeader>
					<CardFrameTitle>Stats</CardFrameTitle>
					<CardFrameDescription>
						Parsed from the model header.
					</CardFrameDescription>
				</CardFrameHeader>
				<Card>
					<CardPanel>
						<Flex.Row className="flex-wrap gap-6">
							{dtype ? <Stat label="Dtype" value={dtype} /> : null}
							{dims ? <Stat label="Rank" value={dims.length} /> : null}
							{params !== null ? (
								<Stat label="Parameters" value={params.toLocaleString()} />
							) : null}
							<Stat
								label="Connections"
								value={(node.edges?.length ?? 0).toLocaleString()}
							/>
						</Flex.Row>
						{dims ? (
							<Flex.Column className="mt-4">
								<ShapeBreakdown dims={dims} />
							</Flex.Column>
						) : null}
					</CardPanel>
				</Card>
			</CardFrame>

			<CardFrame>
				<CardFrameHeader>
					<CardFrameTitle>Structure</CardFrameTitle>
					<CardFrameDescription>
						{isLeaf
							? "This node is a leaf — no children to drill into."
							: "Click any child to drill deeper."}
					</CardFrameDescription>
				</CardFrameHeader>
				<Card>
					<CardPanel className="p-3">
						<ChildrenList
							graph={graph}
							onDrillInto={onDrillInto}
							onNavigate={onSelect}
							parent={selectedName}
						/>
					</CardPanel>
				</Card>
			</CardFrame>
		</Flex.Column>
	);
};
