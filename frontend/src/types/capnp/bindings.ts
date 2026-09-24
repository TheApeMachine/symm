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
BoundText is one bound value as it travelled: the JSON text of the value, not
yet read. A value that is about to be replaced by a later one need never be
parsed.
*/
export interface BoundText {
	graph: string;
	component: string;
	prop: string;
	text: string;
}

/*
readBoundTexts decodes one ui.Bindings frame (nomagique/ui/binding.capnp),
leaving each value as its JSON text.
*/
export function readBoundTexts(buffer: ArrayBuffer | Uint8Array): BoundText[] {
	const message = new MessageReader(buffer);
	const root = message.getRoot(0, 1);

	if (!root) {
		throw new Error(`bindings frame of ${buffer.byteLength} bytes has no root`);
	}

	const list = root.getList(0);

	if (!list) {
		return [];
	}

	const out: BoundText[] = [];

	for (let index = 0; index < list.length; index++) {
		const bound = list.getStruct(index);

		if (!bound) {
			continue;
		}

		out.push({
			graph: bound.getText(0) ?? "",
			component: bound.getText(1) ?? "",
			prop: bound.getText(2) ?? "",
			text: bound.getText(3) ?? "",
		});
	}

	return out;
}

/*
readBound reads a bound value's JSON text.
*/
export function readBound(bound: BoundText): Bound {
	return {
		graph: bound.graph,
		component: bound.component,
		prop: bound.prop,
		value: bound.text === "" ? undefined : JSON.parse(bound.text),
	};
}

/*
readBindings decodes one ui.Bindings frame (nomagique/ui/binding.capnp).
*/
export function readBindings(buffer: ArrayBuffer | Uint8Array): Bound[] {
	return readBoundTexts(buffer).map(readBound);
}
