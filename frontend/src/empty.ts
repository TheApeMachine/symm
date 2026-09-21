const noop = () => {};

export const createConnection = noop;
export class EventEmitter {
	static EventEmitter = EventEmitter;
	on() { return this; }
	once() { return this; }
	off() { return this; }
	emit() { return false; }
	removeListener() { return this; }
	removeAllListeners() { return this; }
}
export class Socket extends EventEmitter {}
export class Duplex extends EventEmitter {}
export class Readable extends EventEmitter {}
export class Writable extends EventEmitter {}

export const randomBytes = (size: number) => {
	const arr = new Uint8Array(size);
	if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
		crypto.getRandomValues(arr);
	}
	return arr;
};
export const createHash = () => ({
	update: () => ({ digest: () => '' }),
	digest: () => ''
});
export const URL = typeof window !== 'undefined' ? window.URL : function() {};

const mockModule: Record<string, unknown> = {
	createConnection,
	EventEmitter,
	Socket,
	Duplex,
	Readable,
	Writable,
	randomBytes,
	createHash,
	URL,
	request: noop,
};

export const createRequire = () => (modName: string) => {
	if (modName === 'events' || modName === 'node:events') {
		return EventEmitter;
	}
	return mockModule;
};
mockModule.createRequire = createRequire as any;

export default mockModule;
