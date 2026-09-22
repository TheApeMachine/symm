import {
	createCollection,
	localStorageCollectionOptions,
} from "@tanstack/react-db";
import { z } from "zod";

/*
PipelineGraphRow is the only persisted shape of a drawn nomagique pipeline.

Everything that has to survive a reload — the nodes, the edges (which live
inside each node's connections), the comments, the viewport — is one row.
Ephemeral view state (drag, hover, selection, edge routing) belongs to
flumeEditorStore instead, because none of it describes the pipeline.

The graph is kept in the browser rather than in the warehouse: a drawing is not
a trading fact, and the hub's storage is for things the run must be able to
account for afterwards. A pipeline that proves worth keeping is promoted by
composing it in Go, not by leaving it in a dashboard's local storage.

schema_version bumps on a breaking shape change, so a row written by an older
editor can be migrated or refused rather than silently misread.
*/
export const PipelineGraphRow = z.object({
	id: z.string().min(1),
	project_id: z.string().nullable(),
	schema_version: z.number().int().default(1),
	nodes: z.record(z.string(), z.unknown()),
	comments: z.record(z.string(), z.unknown()).default({}),
	viewport: z
		.object({
			scale: z.number().default(1),
			translate: z
				.object({
					x: z.number().default(0),
					y: z.number().default(0),
				})
				.default({ x: 0, y: 0 }),
		})
		.default({ scale: 1, translate: { x: 0, y: 0 } }),
	/*
		What this drawing was imported from. The canvas is a working copy of a
		definition the backend holds, so when that definition changes the copy
		is stale — a graph rewired on disk would otherwise keep showing the
		shape it had when it was first opened, with no way to tell.

		null means the row predates this field, or was drawn from nothing.
	*/
	source: z
		.object({
			definition: z.string(),
			fingerprint: z.string(),
		})
		.nullable()
		.default(null),
	updated_at: z.coerce.date(),
});

/*
Identifies a definition by the nodes it holds. Two graphs with the same nodes
wired the same way are the same drawing; anything else is a different one.
*/
export const fingerprintNodes = (nodes: Record<string, unknown>): string => {
	const shape = Object.keys(nodes)
		.sort()
		.map((id) => {
			const node = nodes[id] as {
				type?: string;
				connections?: { inputs?: Record<string, unknown> };
			};

			return `${id}:${node?.type ?? ""}:${Object.keys(node?.connections?.inputs ?? {}).sort().join(",")}`;
		})
		.join("|");

	// FNV-1a: short, stable, and never leaves the browser.
	let hash = 0x811c9dc5;

	for (let index = 0; index < shape.length; index++) {
		hash ^= shape.charCodeAt(index);
		hash = Math.imul(hash, 0x01000193) >>> 0;
	}

	return `${Object.keys(nodes).length}-${hash.toString(16)}`;
};

export type PipelineGraphRowType = z.infer<typeof PipelineGraphRow>;

export const pipelineGraphCollection = createCollection(
	localStorageCollectionOptions({
		id: "pipeline_graphs_local",
		storageKey: "symm:pipeline_graphs",
		schema: PipelineGraphRow,
		getKey: (item) => item.id,
	}),
);
