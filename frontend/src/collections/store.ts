import { createStore, type Store } from "@tanstack/store";
import { Circular, type CircularBuffer } from "./circular";

/*
KeyExtractor derives one nesting key from a row. Every key but the last selects
a nested Record; the final key selects the CircularBuffer the row is pushed into.
*/
export type KeyExtractor<T> = (row: T) => string;

/*
NestedNode is a string-keyed tree of nested maps / leaf buffers. `array` must
also appear in the index union — TypeScript requires every named property to be
assignable to `[key: string]`.
*/
export type NestedNode<T> = {
	[key: string]: NestedNode<T> | CircularBuffer<T> | (() => unknown[]);
	array: () => unknown[];
};

/*
Depth1 is one CircularBuffer per key value (NestedNode with leaf buffers).
*/
export type Depth1<T> = Record<string, CircularBuffer<T>>;

/*
Depth2 nests maps of maps of CircularBuffers (NestedNode of NestedNodes).
*/
export type Depth2<T> = Record<string, Depth1<T>>;

/*
KeyedState is the state every keyed store exposes. `version` is the notify
signal: every write that changes data must add 1 so store.subscribe fires and
can push the buffer over a MessageChannel.
*/
export type KeyedState<Name extends string, Nested> = { version: number } & {
	[K in Name]: Nested;
};

/*
KeyedActions is the action surface every keyed store exposes.
*/
export type KeyedActions<T> = {
	updateFrame: (rows: T[]) => void;
	reset: () => void;
};

/*
Nested builds one nest map with a non-enumerable array() that flattens every
leaf buffer under this node (one level of children may be nested maps themselves).
*/
export const Nested = <T>(): NestedNode<T> => {
	const node = {} as NestedNode<T>;

	Object.defineProperty(node, "array", {
		enumerable: false,
		value: (): unknown[] => {
			const rows: unknown[] = [];

			for (const child of Object.values(node)) {
				if (
					typeof (child as CircularBuffer<T>).values === "function"
				) {
					rows.push(...(child as CircularBuffer<T>).values());
					continue;
				}

				if (typeof (child as NestedNode<T>).array === "function") {
					rows.push(...(child as NestedNode<T>).array());
				}
			}

			return rows;
		},
	});

	return node;
};

/*
FrameStoreState is the widened state shape used when looking up a store by wire
name. Specific stores stay precise at typed call sites; dynamic dispatch needs
one common Store parameter.
*/
export type FrameStoreState = {
	version: number;
	[key: string]: number | NestedNode<unknown> | CircularBuffer<unknown>;
};

/*
FrameStore is TanStack Store widened for string-keyed frameStores lookup.
*/
export type FrameStore = Store<FrameStoreState, KeyedActions<unknown>>;

type ImplState<T> = {
	version: number;
	[key: string]: number | NestedNode<T> | CircularBuffer<T>;
};

/*
push walks nested Records via the key extractors and appends to the leaf buffer.
*/
const push = <T>(
	root: NestedNode<T>,
	keys: Array<KeyExtractor<T>>,
	limit: number,
	row: T,
): void => {
	let node: NestedNode<T> = root;

	for (const extract of keys.slice(0, -1)) {
		const key = extract(row);

		if (key === "") {
			return;
		}

		if (node[key] === undefined) {
			node[key] = Nested<T>();
		}

		node = node[key] as NestedNode<T>;
	}

	const leaf = keys[keys.length - 1]?.(row) ?? "";

	if (leaf === "") {
		return;
	}

	if (node[leaf] === undefined) {
		node[leaf] = Circular<T>(limit);
	}

	const buffer = node[leaf] as CircularBuffer<T>;

	buffer.push(row);
};

/*
createKeyedStore builds one generic frame store. With no key extractors, rows
sit in a single CircularBuffer under `name`. With extractors, every key but the
last nests a Record and the last selects the leaf buffer.
*/
export const createKeyedStore = <T>() => {
	function build<Name extends string>(
		name: Name,
		limit: number,
	): Store<KeyedState<Name, CircularBuffer<T>>, KeyedActions<T>>;

	function build<Name extends string>(
		name: Name,
		limit: number,
		key: KeyExtractor<T>,
	): Store<KeyedState<Name, Depth1<T>>, KeyedActions<T>>;

	function build<Name extends string>(
		name: Name,
		limit: number,
		key1: KeyExtractor<T>,
		key2: KeyExtractor<T>,
	): Store<KeyedState<Name, Depth2<T>>, KeyedActions<T>>;

	function build<Name extends string>(
		name: Name,
		limit: number,
		...keys: Array<KeyExtractor<T>>
	): unknown {
		const flat = keys.length === 0;

		const empty = (): ImplState<T> => ({
			version: 0,
			[name]: flat ? Circular<T>(limit) : Nested<T>(),
		});

		return createStore(empty(), ({ setState }) => ({
			updateFrame: (rows: T[]) =>
				setState((prev) => {
					if (rows.length === 0) {
						return prev;
					}

					if (flat) {
						const buffer = prev[name] as CircularBuffer<T>;

						for (const row of rows) {
							buffer.push(row);
						}

						return {
							...prev,
							[name]: buffer,
							version: prev.version + 1,
						};
					}

					const root = prev[name] as NestedNode<T>;

					for (const row of rows) {
						push(root, keys, limit, row);
					}

					return {
						...prev,
						[name]: root,
						version: prev.version + 1,
					};
				}),
			reset: () => setState(() => empty()),
		})) as Store<
			KeyedState<Name, NestedNode<T> | CircularBuffer<T>>,
			KeyedActions<T>
		>;
	}

	return build;
};
