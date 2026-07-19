export const isPlainObject = (
	value: unknown,
): value is Record<string, unknown> =>
	typeof value === "object" && value !== null && !Array.isArray(value);

const orderedEventArrays = new Set(["findings"]);

/*
mergeFrameEntry coalesces one keyed backend payload inside a 16ms worker window.
Arrays and scalar snapshots replace prior values, while object maps shallow-merge.
*/
export const mergeFrameEntry = (
	existing: unknown,
	incoming: unknown,
): unknown => {
	if (Array.isArray(incoming)) {
		return incoming;
	}

	if (!isPlainObject(incoming)) {
		return incoming;
	}

	if (!isPlainObject(existing)) {
		return incoming;
	}

	return {
		...existing,
		...incoming,
	};
};

/*
mergeFramePayload merges a parsed websocket object into the active worker queue.
Ordered journal evidence is appended so the paint window cannot discard broker
events. Live analysis arrays are complete state snapshots and replace an older
snapshot queued in the same paint window.
*/
export const mergeFramePayload = (
	queue: Record<string, unknown>,
	incoming: Record<string, unknown>,
): Record<string, unknown> => {
	const next = { ...queue };

	for (const [key, data] of Object.entries(incoming)) {
		if (orderedEventArrays.has(key) && Array.isArray(data)) {
			const queued = Array.isArray(queue[key]) ? queue[key] : [];
			next[key] = [...queued, ...data];
			continue;
		}

		next[key] = mergeFrameEntry(queue[key], data);
	}

	return next;
};
