import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";

type DecodeRequest = {
	buffer: ArrayBuffer;
};

type DecodeResponse = {
	frame?: Record<string, unknown>;
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
			if (frame === null) {
				ctx.postMessage({} satisfies DecodeResponse);
				return;
			}

			ctx.postMessage({ frame } satisfies DecodeResponse);
		} catch (error) {
			ctx.postMessage({
				error: error instanceof Error ? error.message : String(error),
			} satisfies DecodeResponse);
		}
	})();
});
