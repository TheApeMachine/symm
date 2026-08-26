/// <reference lib="webworker" />

import { FluidRecordReader } from "#/components/fluid-3d/record";

let peerConnection: RTCPeerConnection | null = null;
let diagnosticsChannel: RTCDataChannel | null = null;

const teardownConnection = () => {
	if (diagnosticsChannel) {
		diagnosticsChannel.close();
		diagnosticsChannel = null;
	}
	if (peerConnection) {
		peerConnection.close();
		peerConnection = null;
	}
};

const setupChannel = (
	channel: RTCDataChannel,
	name: string,
	reader: FluidRecordReader,
) => {
	channel.binaryType = "arraybuffer";

	channel.addEventListener("open", () => {
		self.postMessage({ type: "CHANNEL_OPEN", channel: name });
	});

	channel.addEventListener("close", () => {
		self.postMessage({ type: "CHANNEL_CLOSE", channel: name });
	});

	channel.addEventListener("error", (event) => {
		self.postMessage({ type: "ERROR", error: `webrtc ${name} channel error: ${String(event)}` });
	});

	channel.addEventListener("message", (event: MessageEvent) => {
		try {
			if (!(event.data instanceof ArrayBuffer)) {
				return;
			}

			const record = reader.push(event.data);
			if (record !== null) {
				self.postMessage({ type: "FRAME", channel: name, buffer: record }, [record]);
			}
		} catch (err) {
			self.postMessage({ type: "ERROR", error: String(err) });
		}
	});
};

const connect = async (signalingUrl: string) => {
	try {
		teardownConnection();

		peerConnection = new RTCPeerConnection();

		peerConnection.addEventListener("connectionstatechange", () => {
			if (peerConnection) {
				self.postMessage({
					type: "STATUS",
					status: peerConnection.connectionState,
				});
			}
		});

		diagnosticsChannel = peerConnection.createDataChannel("diagnostics", {
			ordered: true,
		});
		setupChannel(diagnosticsChannel, "diagnostics", new FluidRecordReader());

		const offer = await peerConnection.createOffer();
		await peerConnection.setLocalDescription(offer);

		if (peerConnection.iceGatheringState !== "complete") {
			await new Promise<void>((resolve) => {
				const check = () => {
					if (peerConnection?.iceGatheringState === "complete") {
						peerConnection.removeEventListener("icegatheringstatechange", check);
						resolve();
					}
				};

				peerConnection?.addEventListener("icegatheringstatechange", check);
			});
		}

		const response = await fetch(signalingUrl, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(peerConnection.localDescription),
		});

		if (!response.ok) {
			throw new Error(`Signaling request failed: ${response.statusText}`);
		}

		const answer = await response.json();
		await peerConnection.setRemoteDescription(answer);
	} catch (err) {
		self.postMessage({ type: "ERROR", error: String(err) });
	}
};

const setDiagnostics = async (baseUrl: string, enabled: boolean) => {
	try {
		const response = await fetch(`${baseUrl}/diagnostics`, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ enabled }),
		});
		const result = await response.json();
		self.postMessage({ type: "DIAGNOSTICS_TOGGLE", enabled: result.enabled });
	} catch (err) {
		self.postMessage({ type: "ERROR", error: String(err) });
	}
};

self.postMessage({ type: "READY" });

self.addEventListener("message", (event: MessageEvent) => {
	const message = event.data as {
		type: string;
		url?: string;
		enabled?: boolean;
	};

	switch (message.type) {
		case "CONNECT":
			connect(message.url || "http://127.0.0.1:8765/webrtc/manifold");
			return;
		case "DISCONNECT":
			teardownConnection();
			return;
		case "SET_DIAGNOSTICS":
			setDiagnostics(message.url || "http://127.0.0.1:8765", message.enabled ?? true);
			return;
	}
});
