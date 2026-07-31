import { type ReactNode, useLayoutEffect, useRef } from "react";

type Paint = (updates: unknown) => void;

export type JSONPrimitive = string | number | boolean | null;

export type JSONSerializable =
	| JSONPrimitive
	| JSONSerializable[]
	| { [k: string]: JSONSerializable | undefined };

/*
Component is a wrapper that takes care of boilerplate around the UI.
It is also able to switch from being React managed to being a static
HTML component that is updated via direct DOM manipulation.
This is useful for performance reasons when the component is rapid-fire
updated with real-time data, such as a chart or a table. In those cases,
React's diffing algorithm may introduce unnecessary overhead.

Usage:

<Component
	className="metric-grid"
	register={(paint) =>
		registerPainter(
			"measurements.signalDetail",
			paint,
		)
	}
>
	{({ ref, className }) => (
		<div ref={ref} className={className}>
			<span data-paint="source" />

			<span
				data-paint="raw"
				data-paint-format=".2f"
			/>

			<span
				data-paint="validity.state"
				data-paint-class="valid:text-green-500 invalid:text-red-500"
			/>

			<span
				data-paint="metrics.strength.normalized"
				data-paint-format=".4f"
				data-paint-class="true:font-bold false:font-normal"
			/>
		</div>
	)}
</Component>
*/
interface ComponentRenderProps {
	ref: React.RefObject<HTMLDivElement | null>;
	className?: string;
	slots: number[];
}

interface ComponentProps {
	className?: string;
	register: (paint: Paint) => () => void;
	select?: string;
	children: (props: ComponentRenderProps) => ReactNode;
}

type JSONRecord = { [key: string]: JSONSerializable | undefined };

type PaintDataset = DOMStringMap & {
	paint?: string;
	scope?: string;
	filter?: string;
	paintFormat?: string;
	paintClass?: string;
};

type PaintTarget = {
	key: string;
	element: HTMLElement;
	dataset: PaintDataset;
};

const scanTargets = (root: HTMLElement) => {
	const rootTargets = new Map<string, PaintTarget[]>();

	for (const element of root.querySelectorAll<HTMLElement>("[data-paint]")) {
		const key = element.dataset.paint;
		if (!key) continue;

		const scopedParent = element.closest<HTMLElement>("[data-scope][data-filter]");
		const dataset: PaintDataset = {
			...element.dataset,
			scope: element.dataset.scope ?? scopedParent?.dataset.scope,
			filter: element.dataset.filter ?? scopedParent?.dataset.filter,
		};

		if (!rootTargets.has(key)) {
			rootTargets.set(key, []);
		}

		rootTargets.get(key)?.push({
			key,
			element,
			dataset,
		});
	}

	return rootTargets;
};

const formatValue = (value: unknown, format: string | undefined): string => {
	if (format) {
		switch (typeof value) {
			case "number":
				return value.toFixed(parseInt(format.slice(1), 10));
			case "boolean":
				return value ? "true" : "false";
		}
	}

	return String(value);
};

const readPath = (
	updates: JSONSerializable,
	path: string | undefined,
): JSONSerializable | undefined => {
	if (!path) {
		return updates;
	}

	const parts = path.split(".");
	let value: JSONSerializable | undefined = updates;

	for (const part of parts) {
		if (value === undefined || value === null) {
			return undefined;
		}

		if (typeof value === "object" && !Array.isArray(value)) {
			value = (value as JSONRecord)[part];
		} else {
			return undefined;
		}
	}

	return value;
};

const selectScopedUpdates = (
	updates: JSONSerializable,
	dataset: PaintDataset,
): JSONSerializable | undefined => {
	if (!Array.isArray(updates)) {
		return updates;
	}

	if (!dataset.scope || dataset.filter === undefined) {
		return updates[0];
	}

	for (const item of updates) {
		if (item === null || typeof item !== "object" || Array.isArray(item)) {
			continue;
		}

		const scopedValue = readPath(item as JSONSerializable, dataset.scope);

		if (scopedValue === dataset.filter) {
			return item;
		}
	}

	return undefined;
};

const applyPaintClass = (element: HTMLElement, value: JSONSerializable): void => {
	const spec = element.dataset.paintClass;

	if (!spec) {
		return;
	}

	for (const rule of spec.split(/\s+/)) {
		const separator = rule.indexOf(":");

		if (separator === -1) {
			continue;
		}

		const expected = rule.slice(0, separator);
		const className = rule.slice(separator + 1);

		if (!className) {
			continue;
		}

		element.classList.toggle(className, String(value) === expected);
	}
};

const updateTargets = (
	targets: Map<string, PaintTarget[]>,
	updates: JSONSerializable,
) => {
	if (updates === undefined || updates === null) return;

	for (const [key, targetsByKey] of targets) {
		for (const target of targetsByKey) {
			const scopedUpdates = selectScopedUpdates(updates, target.dataset);

			if (scopedUpdates === undefined || scopedUpdates === null) {
				continue;
			}

			const value = readPath(scopedUpdates, key);

			if (value === undefined || value === null) {
				continue;
			}

			target.element.textContent = formatValue(value, target.dataset.paintFormat);
			applyPaintClass(target.element, value);
		}
	}
};

export const Component = ({
	className,
	register,
	select,
	children,
}: ComponentProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const slots = useRef<number[]>([]);

	useLayoutEffect(() => {
		if (!ref.current) return;
		const rootTargets = scanTargets(ref.current);

		const paint = (updates: JSONSerializable) => {
			// Ignore undefined or null updates (and later also empty values)
			// to avoid "flickering" or "state" loss. Remember, the static DOM
			// is our "retained" state, so we must not overwrite it, only
			// update or append.
			if (updates === undefined || updates === null) return;

			if (select && updates) {
				// When a select key is provided, we scope the updated data to
				// that key, which allows for sub-selections as a prop on the
				// Component. This is useful for cases where the data structure
				// is nested and we only want to update a specific part of it.
				const selectedUpdates = readPath(updates, select);

				if (selectedUpdates === undefined || selectedUpdates === null) {
					return;
				}

				updates = selectedUpdates;
			}

			updateTargets(rootTargets, updates);
		};

		const unregister = register(paint as Paint);

		return () => {
			unregister?.();
		};
	}, [register, select]);

	return children({
		ref,
		className,
		slots: slots.current,
	});
};
