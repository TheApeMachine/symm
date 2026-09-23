import { MessageReader } from "@naeemo/capnp";
// The composite-list decoding fix lives with the first reader written here.
import "./measurement";

/*
Bound is one value that reached a component's input port in the running
program, addressed by the graph and node id the component was authored with.
*/
export interface Bound {
	graph: string;
	component: string;
	prop: string;
	value: unknown;
}

/*
readBindings decodes one ui.Bindings frame (nomagique/ui/binding.capnp).
*/
export function readBindings(buffer: ArrayBuffer | Uint8Array): Bound[] {
	const message = new MessageReader(buffer);
	const root = message.getRoot(0, 1);

	if (!root) {
		throw new Error(`bindings frame of ${buffer.byteLength} bytes has no root`);
	}

	const list = root.getList(0);

	if (!list) {
		return [];
	}

	const out: Bound[] = [];

	for (let index = 0; index < list.length; index++) {
		const bound = list.getStruct(index);

		if (!bound) {
			continue;
		}

		const text = bound.getText(3) ?? "";

		out.push({
			graph: bound.getText(0) ?? "",
			component: bound.getText(1) ?? "",
			prop: bound.getText(2) ?? "",
			value: text === "" ? undefined : JSON.parse(text),
		});
	}

	return out;
}
