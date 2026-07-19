import {
	type DependencyList,
	useId,
	useLayoutEffect,
	useRef,
} from "react";

type Watch = {
	store: string;
	key: string;
};

/*
useDirectStorePaint opens worker MessageChannels for the given store keys.
Writes in the worker bump store.version, which fires store.subscribe and pushes
the circular buffer over the port. This hook paints those pushes on rAF.
Inline paint callbacks are ref-stable so they cannot re-subscribe every render.
*/
export const useDirectStorePaint = (
	worker: Worker | null,
	watches: Watch[],
	paint: (buffers: Record<string, unknown[]>) => void,
	deps: DependencyList,
) => {
	const instanceId = useId();
	const paintRef = useRef(paint);
	paintRef.current = paint;

	const watchesKey = watches
		.map((watch) => `${watch.store}\0${watch.key}`)
		.join("\n");

	useLayoutEffect(() => {
		if (worker === null) {
			return;
		}

		let frame: number | null = null;
		const buffers: Record<string, unknown[]> = {};
		const ports: MessagePort[] = [];

		const specs = watchesKey
			.split("\n")
			.filter(Boolean)
			.map((entry) => {
				const split = entry.indexOf("\0");
				const store = entry.slice(0, split);
				const key = entry.slice(split + 1);
				const bufferKey = `${store}:${key}`;

				return {
					store,
					key,
					bufferKey,
					id: `${instanceId}:${bufferKey}`,
				};
			});

		const schedule = () => {
			if (frame !== null) {
				return;
			}

			frame = requestAnimationFrame(() => {
				frame = null;
				paintRef.current(buffers);
			});
		};

		const onWorker = (event: MessageEvent) => {
			if (event.data?.type !== "SUBSCRIBED") {
				return;
			}

			const id = event.data.id as string;
			const spec = specs.find((entry) => entry.id === id);

			if (spec === undefined) {
				return;
			}

			const port = event.ports[0];
			ports.push(port);

			port.onmessage = (message) => {
				const rows = message.data as unknown[];

				buffers[spec.bufferKey] = Array.isArray(rows) ? rows : [];
				schedule();
			};
		};

		worker.addEventListener("message", onWorker);

		for (const spec of specs) {
			buffers[spec.bufferKey] = [];
			worker.postMessage({
				type: "SUBSCRIBE",
				id: spec.id,
				store: spec.store,
				key: spec.key,
			});
		}

		paintRef.current(buffers);

		return () => {
			worker.removeEventListener("message", onWorker);

			for (const spec of specs) {
				worker.postMessage({ type: "UNSUBSCRIBE", id: spec.id });
			}

			for (const port of ports) {
				port.close();
			}

			if (frame !== null) {
				cancelAnimationFrame(frame);
			}
		};
		// eslint-disable-next-line react-hooks/exhaustive-deps -- watchesKey / paintRef
	}, [worker, watchesKey, instanceId, ...deps]);
};
