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
	schemaVersions?: Record<string, string>;
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

export type HindsightGap = {
	runId: string;
	encoding: string;
	sequence: number;
	detail: string;
};

export type HindsightLifecycleEvent = {
	decisionId: string;
	symbol: string;
	kind: string;
	action: string;
	at: string;
};

export type HindsightEnvelope = {
	run: string;
	sequence: number;
	payload: string;
	manifests: Array<{
		envelope: {
			origin: HindsightCaptureIdentity;
			ordinal: number;
		};
		workload: string;
		domainKind: string;
		symbol: string;
	}>;
	witnesses: Array<{
		envelope: {
			origin: HindsightCaptureIdentity;
			ordinal: number;
		};
		boundary: string;
		artifact: { kind: string; identity: string };
		component: string;
		componentStateVersion: number;
		immediateParents: Array<{
			origin: HindsightCaptureIdentity;
			ordinal: number;
		}>;
	}>;
};
