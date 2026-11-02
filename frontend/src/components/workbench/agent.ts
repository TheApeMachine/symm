/*
Agent configures Perspective's embedded LLM chat on a viewer element.

Perspective keeps the Chat tab hidden until agentConfig runs. The React wrapper
does not forward a ref, so the workbench finds the custom element in the DOM
and hands it here.
*/

type DocsBundle = {
	schemas?: unknown;
	chunks: Array<{ title?: string; path?: string; text: string }>;
};

type AgentViewer = {
	agentConfig: (config: unknown) => void;
};

const DATASET_DOCS = [
	{
		title: "Symm warehouse tables",
		text:
			"Tables include open_* and closed_* lot views plus level3_frames and " +
			"level3_events. Lot columns: symbol, entry_path, exit_path, " +
			"exit_trigger, opened_at, closed_at, pnl, pnl_pct, mark, " +
			"entry_order, exit_order, fills. Qty and entry price come from " +
			"fills / entry_order.volume — not separate scalar columns. " +
			"pnl_pct is percentage points. level3_events has symbol, bids, " +
			"asks, checksum, frame_type.",
	},
];

/*
Agent applies the LM Studio connection to a mounted <perspective-viewer>.
*/
export const Agent = {
	async configure(viewer: AgentViewer): Promise<void> {
		const [{ providers }, docsModule] = await Promise.all([
			import("@perspective-dev/viewer"),
			import(
				"@perspective-dev/viewer/dist/docs/perspective-docs.json"
			),
		]);

		const docs = (docsModule.default ?? docsModule) as DocsBundle;

		viewer.agentConfig({
			...providers.lmstudio,
			apiKey: "sk-lm-4WfBrPQs:PvpDN8tHQeihIovEikA3",
			model: "qwen3.8-27b-splash",
			docs: {
				...docs,
				chunks: [...docs.chunks, ...DATASET_DOCS],
			},
		});
	},
};
