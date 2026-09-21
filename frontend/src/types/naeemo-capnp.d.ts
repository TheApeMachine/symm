declare module "@naeemo/capnp" {
	export class MessageReader {
		constructor(buffer: ArrayBuffer);
		getRoot(dataWords?: number, pointerWords?: number): any;
	}
}
