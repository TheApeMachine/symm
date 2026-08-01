import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { registerPainter } from "#/providers/ws-stores";
import type { JSONSerializable } from "./paint";

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
	registerKey="measurements.signalDetail"
>
	{({ ref, className, slots }) => (
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

			{slots.map((index) => (
				<div key={index} data-index={index} />
			))}
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
	registerKey?: string;
	select?: string;
	children: (props: ComponentRenderProps) => ReactNode;
}

type JSONRecord = { [key: string]: JSONSerializable | undefined };

type PaintDataset = DOMStringMap & {
	paint?: string;
	paintProp?: string;
	set?: string;
	target?: string;
	scope?: string;
	filter?: string;
	index?: string;
	paintFormat?: string;
	paintClass?: string;
};

type PaintBinding = {
	key: string;
	element: HTMLElement;
	dataset: PaintDataset;
	mode: "paint" | "set";
};

const scanTargets = (root: HTMLElement) => {
	const rootTargets = new Map<string, PaintBinding[]>();

	for (const element of root.querySelectorAll<HTMLElement>("[data-paint], [data-set]")) {
		const key = element.dataset.paint ?? element.dataset.set;

		if (!key) {
			continue;
		}

		const scopedParent = element.closest<HTMLElement>("[data-scope][data-filter]");
		const dataset: PaintDataset = {
			...element.dataset,
			scope: element.dataset.scope ?? scopedParent?.dataset.scope,
			filter: element.dataset.filter ?? scopedParent?.dataset.filter,
			index: element.dataset.index ?? element.closest<HTMLElement>("[data-index]")?.dataset.index,
		};

		if (!rootTargets.has(key)) {
			rootTargets.set(key, []);
		}

		rootTargets.get(key)?.push({
			key,
			element,
			dataset,
			mode: element.dataset.set ? "set" : "paint",
		});
	}

	return rootTargets;
};

const formatValue = (value: unknown, format: string | undefined): string => {
	if (format) {
		switch (typeof value) {
			case "number": {
				if (!/^\.\d+f?$/i.test(format)) {
					throw new Error(`invalid data-paint-format: ${format}`);
				}

				const digits = Number.parseInt(format.slice(1), 10);

				if (!Number.isInteger(digits)) {
					throw new Error(`invalid data-paint-format: ${format}`);
				}

				if (digits < 0 || digits > 100) {
					throw new Error(
						`data-paint-format fractional digits out of range: ${format}`,
					);
				}

				return value.toFixed(digits);
			}
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

		if (Array.isArray(value)) {
			const index = Number.parseInt(part, 10);

			if (!Number.isInteger(index)) {
				return undefined;
			}

			value = value[index];
			continue;
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
		const index = Number.parseInt(dataset.index ?? "", 10);

		if (Number.isInteger(index)) {
			return updates[index];
		}

		return undefined;
	}

	for (const item of updates) {
		if (item === null || typeof item !== "object" || Array.isArray(item)) {
			continue;
		}

		const scopedValue = readPath(item as JSONSerializable, dataset.scope);

		if (String(scopedValue) === dataset.filter) {
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

	const splitClassList = (classList: string) => {
		const tokens: string[] = [];
		let depth = 0;
		let start = 0;

		for (let index = 0; index < classList.length; index += 1) {
			const char = classList[index];

			if (char === "[" || char === "(") {
				depth += 1;
			}

			if ((char === "]" || char === ")") && depth > 0) {
				depth -= 1;
			}

			if (char !== "," || depth !== 0) {
				continue;
			}

			tokens.push(classList.slice(start, index));
			start = index + 1;
		}

		tokens.push(classList.slice(start));

		return tokens;
	};

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

		for (const token of splitClassList(className)) {
			if (!token) {
				continue;
			}

			element.classList.toggle(token, String(value) === expected);
		}
	}
};

const setTargetValue = (
	element: HTMLElement,
	target: string | undefined,
	value: JSONSerializable,
) => {
	if (!target) {
		return;
	}

	const parts = target.split(".");
	let current: unknown = element;

	for (const part of parts.slice(0, -1)) {
		if (current === null || current === undefined || typeof current !== "object") {
			return;
		}

		current = (current as Record<string, unknown>)[part];
	}

	const property = parts.at(-1);

	if (!property || current === null || current === undefined || typeof current !== "object") {
		return;
	}

	(current as Record<string, unknown>)[property] = String(value);
};

const updateTargets = (
	targets: Map<string, PaintBinding[]>,
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

			if (target.mode === "set") {
				setTargetValue(target.element, target.dataset.target, value);
				continue;
			}

			let formatted: string;

			try {
				formatted = formatValue(value, target.dataset.paintFormat);
			} catch {
				continue;
			}

			if (target.dataset.paintProp) {
				setTargetValue(target.element, target.dataset.paintProp, formatted);
			} else {
				target.element.textContent = formatted;
			}

			applyPaintClass(target.element, value);
		}
	}
};

export const Component = ({
	className,
	registerKey,
	select,
	children,
}: ComponentProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const latest = useRef<JSONSerializable | undefined>(undefined);
	const [slots, setSlots] = useState<number[]>([]);

	useLayoutEffect(() => {
		if (!ref.current) return;
		const rootTargets = scanTargets(ref.current);

		const paint = (updates: JSONSerializable) => {
			latest.current = updates;

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

			if (Array.isArray(updates)) {
				const indexedTargets = Array.from(rootTargets.values())
					.flat()
					.filter((target) => Number.isInteger(Number.parseInt(target.dataset.index ?? "", 10)));

				if (indexedTargets.length > 0 && slots.length < updates.length) {
					setSlots(Array.from({ length: updates.length }, (_, index) => index));
					return;
				}
			}

			updateTargets(rootTargets, updates);
		};

		if (!registerKey) {
			return;
		}

		const unregister = registerPainter(registerKey, paint);

		if (latest.current !== undefined) {
			paint(latest.current);
		}

		return () => {
			unregister?.();
		};
	}, [registerKey, select, slots]);

	return children({
		ref,
		className,
		slots,
	});
};
