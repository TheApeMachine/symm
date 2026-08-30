/*
Hindsight read-model types: the capture tape and its persisted historical
states, exactly as the hub's /hindsight/* endpoints project the store records.
These mirror hindsight.CaptureIdentity / Run / StateEntry — not a parallel
domain model — so the UI reads the same identities the backend persisted.
*/

export type HindsightRun = {
	id: string;
	startedAt: string;
	codeCommit: string;
	buildId: string;
	configDigest: string;
	integrity: "COMPLETE" | "GAPPED" | "CORRUPT" | "UNKNOWN";
};

export type HindsightCaptureIdentity = {
	run: string;
	sequence: number;
	stream: string;
	streamEpoch: number;
	streamSequence: number;
};

export type HindsightCapture = {
	identity: HindsightCaptureIdentity;
	kind: string;
	endpoint: string;
	receivedAt: string;
};

export type HindsightState = {
	envelope: {
		origin: HindsightCaptureIdentity;
		ordinal: number;
	};
	payload: string;
};
