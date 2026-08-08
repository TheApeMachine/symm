const RECORD_HEADER_BYTES = 8;
const RECORD_MAGIC = new Uint8Array([0x53, 0x46, 0x44, 0x31]);

/*
FluidRecordReader reassembles one ordered WebRTC record without interpreting or
reshaping the JSON value carried inside it.
*/
export class FluidRecordReader {
	private expectedBytes = 0;
	private offset = 0;
	private record: Uint8Array | null = null;

	push(message: ArrayBuffer): ArrayBuffer | null {
		const bytes = new Uint8Array(message);

		if (this.record === null) {
			this.begin(bytes);
			return null;
		}

		if (this.offset + bytes.byteLength > this.expectedBytes) {
			this.reset();
			throw new Error("fluid WebRTC record exceeded its declared length");
		}

		this.record.set(bytes, this.offset);
		this.offset += bytes.byteLength;

		if (this.offset !== this.expectedBytes) {
			return null;
		}

		const complete = this.record.buffer as ArrayBuffer;
		this.reset();
		return complete;
	}

	private begin(header: Uint8Array) {
		if (header.byteLength !== RECORD_HEADER_BYTES) {
			throw new Error("fluid WebRTC record header must contain 8 bytes");
		}

		for (let index = 0; index < RECORD_MAGIC.length; index += 1) {
			if (header[index] !== RECORD_MAGIC[index]) {
				throw new Error("fluid WebRTC record has an invalid SFD1 header");
			}
		}

		this.expectedBytes = new DataView(header.buffer).getUint32(4, true);

		if (this.expectedBytes === 0) {
			throw new Error("fluid WebRTC record cannot be empty");
		}

		this.record = new Uint8Array(this.expectedBytes);
		this.offset = 0;
	}

	private reset() {
		this.expectedBytes = 0;
		this.offset = 0;
		this.record = null;
	}
}
