declare module "@naeemo/capnp" {
	export enum PointerTag {
		STRUCT = 0,
		LIST = 1,
		FAR = 2,
		OTHER = 3,
	}

	export enum ElementSize {
		VOID = 0,
		BIT = 1,
		BYTE = 2,
		TWO_BYTES = 3,
		FOUR_BYTES = 4,
		EIGHT_BYTES = 5,
		POINTER = 6,
		COMPOSITE = 7,
	}

	export interface StructSize {
		dataWords: number;
		pointerCount: number;
	}

	export interface SecurityOptions {
		maxTotalSize?: number;
		maxSegments?: number;
		strictMode?: boolean;
	}

	export class Segment {
		buffer: ArrayBuffer;
		view: DataView;
		constructor(initialCapacity?: number);
		static fromBuffer(buffer: ArrayBuffer): Segment;
		getWord(wordOffset: number): bigint;
		setWord(wordOffset: number, value: bigint): void;
		asUint8Array(): Uint8Array;
		getArrayBuffer(): ArrayBuffer;
		get wordCount(): number;
		get byteLength(): number;
		get dataView(): DataView;
	}

	export interface ResolvedPointer {
		segmentIndex: number;
		wordOffset: number;
		pointer: {
			tag: PointerTag;
			offset?: number;
			dataWords?: number;
			pointerCount?: number;
			elementSize?: ElementSize;
			elementCount?: number;
			targetSegment?: number;
			targetOffset?: number;
			doubleFar?: boolean;
		};
	}

	export class MessageReader {
		segments: Segment[];
		securityOptions: SecurityOptions;
		constructor(buffer: ArrayBuffer | Uint8Array, securityOptions?: SecurityOptions);
		getRoot(dataWords?: number, pointerWords?: number): StructReader;
		getSegment(index: number): Segment | undefined;
		resolvePointer(segmentIndex: number, wordOffset: number): ResolvedPointer | null;
		get segmentCount(): number;
		getSecurityOptions(): SecurityOptions;
	}

	export class StructReader {
		message: MessageReader;
		segmentIndex: number;
		wordOffset: number;
		dataWords: number;
		pointerCount: number;
		constructor(
			message: MessageReader,
			segmentIndex: number,
			wordOffset: number,
			dataWords: number,
			pointerCount: number,
		);
		getBool(byteOffset: number, defaultVal?: boolean): boolean;
		getInt8(byteOffset: number, defaultVal?: number): number;
		getInt16(byteOffset: number, defaultVal?: number): number;
		getInt32(byteOffset: number, defaultVal?: number): number;
		getInt64(byteOffset: number): bigint;
		getUint8(byteOffset: number, defaultVal?: number): number;
		getUint16(byteOffset: number, defaultVal?: number): number;
		getUint32(byteOffset: number, defaultVal?: number): number;
		getUint64(byteOffset: number): bigint;
		getFloat32(byteOffset: number, defaultVal?: number): number;
		getFloat64(byteOffset: number, defaultVal?: number): number;
		getText(pointerIndex: number, defaultVal?: string): string;
		getData(pointerIndex: number): Uint8Array | undefined;
		getStruct(pointerIndex: number, dataWords?: number, pointerCount?: number): StructReader | undefined;
		getList(pointerIndex: number, elementSize?: ElementSize, structSize?: StructSize): ListReader | undefined;
	}

	export class ListReader {
		message: MessageReader;
		segmentIndex: number;
		elementSize: ElementSize;
		elementCount: number;
		structSize?: StructSize;
		startOffset: number;
		constructor(
			message: MessageReader,
			segmentIndex: number,
			elementSize: ElementSize,
			elementCount: number,
			structSize?: StructSize,
			startOffset?: number,
		);
		get length(): number;
		getPrimitive(index: number): number | bigint;
		getStruct(index: number): StructReader;
	}

	export class MessageBuilder {
		constructor();
		initRoot(dataWords: number, pointerCount: number): StructBuilder;
		toArrayBuffer(): ArrayBuffer;
		getSegment(): Segment;
	}

	export class StructBuilder {
		setBool(byteOffset: number, value: boolean): void;
		setInt8(byteOffset: number, value: number): void;
		setInt16(byteOffset: number, value: number): void;
		setInt32(byteOffset: number, value: number): void;
		setInt64(byteOffset: number, value: bigint): void;
		setUint8(byteOffset: number, value: number): void;
		setUint16(byteOffset: number, value: number): void;
		setUint32(byteOffset: number, value: number): void;
		setUint64(byteOffset: number, value: bigint): void;
		setFloat32(byteOffset: number, value: number): void;
		setFloat64(byteOffset: number, value: number): void;
		setText(pointerIndex: number, value: string): void;
		setData(pointerIndex: number, value: Uint8Array): void;
		initStruct(pointerIndex: number, dataWords: number, pointerCount: number): StructBuilder;
		initList(pointerIndex: number, elementSize: ElementSize, count: number, structSize?: StructSize): ListBuilder;
	}

	export class ListBuilder {
		get length(): number;
		setPrimitive(index: number, value: number | bigint): void;
		getStruct(index: number): StructBuilder;
	}
}
