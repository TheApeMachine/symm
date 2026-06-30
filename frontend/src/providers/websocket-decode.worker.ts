import type { ArtifactFrame } from "#/collections/artifacts";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";

type DecodeRequest = {
	buffer: ArrayBuffer;
};

type DecodeResponse = {
	frame: ArtifactFrame | null;
	error?: string;
};

type DecodeWorkerScope = {
	addEventListener: (
		type: "message",
		listener: (event: MessageEvent<DecodeRequest>) => void,
	) => void;
	postMessage: (message: DecodeResponse) => void;
};

const ctx = self as unknown as DecodeWorkerScope;

ctx.addEventListener("message", (event: MessageEvent<DecodeRequest>) => {
	void (async () => {
		try {
			const frame = await decodePackedArtifactWire(event.data.buffer);
			ctx.postMessage({ frame } satisfies DecodeResponse);
		} catch (error) {
			ctx.postMessage({
				frame: null,
				error: error instanceof Error ? error.message : String(error),
			} satisfies DecodeResponse);
		}
	})();
});
