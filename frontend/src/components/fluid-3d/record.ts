const CHUNK_HEADER_BYTES = 16;
const RECORD_MAGIC = new Uint8Array([0x53, 0x46, 0x44, 0x31]);

type PendingFrame = {
	frameId: number;
	chunkCount: number;
	chunks: Map<number, Uint8Array>;
	totalBytes: number;
};

/*
FluidRecordReader reassembles one self-identifying chunked WebRTC frame.

Every SCTP message carries a 16-byte header — magic, frame ID, chunk index,
chunk count — so unordered, non-retransmitting delivery can reassemble exactly
one complete frame and discard incomplete or obsolete frames the moment a
newer frame is observed.
*/
export class FluidRecordReader {
	private latestFrameId = -1;
	private pending: PendingFrame | null = null;

	push(message: ArrayBuffer): ArrayBuffer | null {
		const bytes = new Uint8Array(message);

		if (bytes.byteLength < CHUNK_HEADER_BYTES) {
			throw new Error("fluid WebRTC chunk is shorter than its 16-byte header");
		}

		for (let index = 0; index < RECORD_MAGIC.length; index += 1) {
			if (bytes[index] !== RECORD_MAGIC[index]) {
				throw new Error("fluid WebRTC chunk has an invalid SFD1 header");
			}
		}

		const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
		const frameId = view.getUint32(4, true);
		const chunkIndex = view.getUint32(8, true);
		const chunkCount = view.getUint32(12, true);

		if (chunkCount === 0) {
			throw new Error("fluid WebRTC frame cannot have zero chunks");
		}

		if (frameId < this.latestFrameId) {
			// An obsolete frame arrived after its successor: discard it.
			return null;
		}

		if (frameId > this.latestFrameId) {
			this.latestFrameId = frameId;
			this.pending = {
				frameId,
				chunkCount,
				chunks: new Map(),
				totalBytes: 0,
			};
		}

		const pending = this.pending;

		if (pending === null || pending.frameId !== frameId) {
			return null;
		}

		if (chunkIndex >= chunkCount || pending.chunks.has(chunkIndex)) {
			return null;
		}

		const payload = bytes.slice(CHUNK_HEADER_BYTES);
		pending.chunks.set(chunkIndex, payload);
		pending.totalBytes += payload.byteLength;

		if (pending.chunks.size !== pending.chunkCount) {
			return null;
		}

		return this.finish(pending);
	}

	private finish(frame: PendingFrame): ArrayBuffer {
		const assembled = new Uint8Array(frame.totalBytes);
		let offset = 0;

		for (let index = 0; index < frame.chunkCount; index += 1) {
			const chunk = frame.chunks.get(index);

			if (!chunk) {
				throw new Error(
					`fluid WebRTC frame ${frame.frameId} is missing chunk ${index}`,
				);
			}

			assembled.set(chunk, offset);
			offset += chunk.byteLength;
		}

		this.pending = null;

		return assembled.buffer as ArrayBuffer;
	}
}
