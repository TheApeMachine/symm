"use client";

import {
	GitCommitVerticalIcon,
	SparklesIcon,
	SplineIcon,
	WaypointsIcon,
} from "lucide-react";
import { useMemo, useRef } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { ToggleGroup, ToggleGroupItem } from "#/components/ui/toggle-group";
import { Typography } from "#/components/ui/typography";
import { useOperations } from "#/service/compute";
import { buildFlumeConfigFromSchemas } from "./build-config-from-schemas";
import type { EdgeRoutingMode } from "./connectionCalculator";
import { setRoutingMode, useRoutingMode } from "./flume-editor.store";
import { NodeEditor, type NodeEditorHandle } from "./NodeEditor";

const LOCAL_GRAPH_ID = "local-default";

const EXAMPLE_NODES = [
	{ type: "source", x: 120, y: 180 },
	{ type: "gate", x: 420, y: 180 },
	{ type: "sink", x: 720, y: 180 },
] as const;

const EXAMPLE_CONNECTIONS = [
	{
		output: { nodeType: "source", portName: "value" },
		input: { nodeType: "gate", portName: "in" },
	},
	{
		output: { nodeType: "gate", portName: "out" },
		input: { nodeType: "sink", portName: "value" },
	},
] as const;

const ROUTING_OPTIONS: ReadonlyArray<{
	value: EdgeRoutingMode;
	label: string;
	icon: typeof SplineIcon;
	hint: string;
}> = [
	{ value: "smooth", label: "Smooth", icon: SplineIcon, hint: "Curved bezier" },
	{
		value: "straight",
		label: "Straight",
		icon: GitCommitVerticalIcon,
		hint: "Direct line",
	},
	{
		value: "orthogonal",
		label: "Orthogonal",
		icon: WaypointsIcon,
		hint: "Right-angle, A*-routed",
	},
];

const EdgeRoutingToggle = ({
	value,
	onChange,
}: {
	value: EdgeRoutingMode;
	onChange: (next: EdgeRoutingMode) => void;
}) => {
	return (
		<Flex.Row className="items-center gap-2">
			<Typography.Span className="text-xs" variant="muted">
				Edges
			</Typography.Span>
			<ToggleGroup
				onValueChange={(next) => {
					const candidate = next[0];

					if (
						candidate === "smooth" ||
						candidate === "straight" ||
						candidate === "orthogonal"
					) {
						onChange(candidate);
					}
				}}
				size="sm"
				value={[value]}
				variant="outline"
			>
				{ROUTING_OPTIONS.map((option) => {
					const Icon = option.icon;

					return (
						<ToggleGroupItem
							aria-label={option.hint}
							key={option.value}
							title={option.hint}
							value={option.value}
						>
							<Icon className="size-3.5" />
							<span className="hidden sm:inline">{option.label}</span>
						</ToggleGroupItem>
					);
				})}
			</ToggleGroup>
		</Flex.Row>
	);
};

type FlumeEditorProps = {
	projectId?: string;
};

export const FlumeEditor = ({ projectId }: FlumeEditorProps) => {
	const { data: operations, isPending, isError, isSuccess } = useOperations();
	const graphId = projectId ?? LOCAL_GRAPH_ID;
	const routingMode = useRoutingMode();
	const editorHandleRef = useRef<NodeEditorHandle | null>(null);

	const flumeConfig = useMemo(
		() => buildFlumeConfigFromSchemas(operations ?? {}),
		[operations],
	);

	const editorMode = isError || !isSuccess ? "builtin-only" : "full";

	const insertExample = () => {
		editorHandleRef.current?.seed({
			defaultNodes: [...EXAMPLE_NODES],
			defaultConnections: [...EXAMPLE_CONNECTIONS],
		});
	};

	if (isPending) {
		return (
			<div className="flex min-h-[75vh] flex-1 items-center justify-center text-muted-foreground text-sm">
				Loading operation schemas…
			</div>
		);
	}

	return (
		<div className="flex min-h-[75vh] flex-1 flex-col gap-3">
			<Flex.Row className="shrink-0 items-center justify-between gap-3 rounded-xl border bg-muted/48 px-3 py-2">
				<Flex.Row className="items-center gap-3">
					{isError ? (
						<Typography.Span className="text-xs" variant="muted">
							Built-in node types only — backend operations unavailable.
						</Typography.Span>
					) : (
						<Typography.Span className="text-xs" variant="muted">
							{Object.keys(flumeConfig.nodeTypes).length} node types
						</Typography.Span>
					)}
					<Button
						onClick={insertExample}
						size="sm"
						title="Seed a Source → Gate → Sink example (idempotent — no-op if the graph already has nodes)"
						type="button"
						variant="ghost"
					>
						<SparklesIcon />
						Insert example
					</Button>
				</Flex.Row>
				<EdgeRoutingToggle onChange={setRoutingMode} value={routingMode} />
			</Flex.Row>
			<NodeEditor
				key={`${editorMode}:${graphId}`}
				className="min-h-0 flex-1"
				edgeRoutingMode={routingMode}
				graphId={graphId}
				nodeTypes={flumeConfig.nodeTypes}
				portTypes={flumeConfig.portTypes}
				projectId={projectId ?? null}
				ref={editorHandleRef}
				style={{ minHeight: isError ? "70vh" : "75vh" }}
			/>
		</div>
	);
};
