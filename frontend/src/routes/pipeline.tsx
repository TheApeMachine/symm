import { createFileRoute } from "@tanstack/react-router";
import { FlumeEditor } from "#/components/flume/flume-editor";
import { Toasts } from "#/components/ui/toast";

/*
The pipeline surface is where a nomagique composition is drawn rather than
written. Its palette is the library's own primitives, read from the hub's
generated catalog, so the editor can only offer what nomagique actually has.
*/
const RouteComponent = () => (
	<div className="flex h-full min-h-0 flex-col">
		<FlumeEditor />
		<Toasts />
	</div>
);

export const Route = createFileRoute("/pipeline")({
	component: RouteComponent,
});
