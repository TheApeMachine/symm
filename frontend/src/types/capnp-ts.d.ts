declare module "capnp-ts" {
	export type ObjectSize = unknown;
	export type Orphan<T> = unknown;
	export type List<T> = unknown;
	export type ListCtor<T> = unknown;

	export class Data {
		toUint8Array(): Uint8Array;
		copyBuffer(buffer: Uint8Array): void;
	}

	export class Struct {}

	export class Message {
		constructor(buffer?: ArrayBuffer, packed?: boolean);
		initRoot<T>(root: new (...args: never[]) => T): T;
		getRoot<T>(root: new (...args: never[]) => T): T;
		toPackedArrayBuffer(): ArrayBuffer;
	}

	export class Int64 {
		static fromNumber(value: number): Int64;
		toString(): string;
	}
}
