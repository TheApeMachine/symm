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
	updated_at: z.coerce.date(),
});

export type PipelineGraphRowType = z.infer<typeof PipelineGraphRow>;

export const pipelineGraphCollection = createCollection(
	localStorageCollectionOptions({
		id: "pipeline_graphs_local",
		storageKey: "symm:pipeline_graphs",
		schema: PipelineGraphRow,
		getKey: (item) => item.id,
	}),
);
