import { type ReactNode, useLayoutEffect, useRef } from "react";

type Paint = (updates: unknown) => void;

type PaintTarget = {
	elements: PaintElement[];
	path: string[];
};

type PaintElement = {
	element: HTMLElement;
	format: (value: unknown) => string;
	classes: Map<string, string[]>;
	classNames: string[];
};

interface ComponentRenderProps {
	ref: React.RefObject<HTMLDivElement | null>;
	className?: string;
}

interface ComponentProps {
	className?: string;
	register: (paint: Paint) => () => void;
	children: (props: ComponentRenderProps) => ReactNode;
}

/*
getValue resolves a data-paint path against an update object.

Examples:

data-paint="source"
data-paint="validity.state"
data-paint="uncertainty.confidence"
data-paint="metrics.strength.normalized"
*/
const getValue = (
	updates: Record<string, unknown>,
	path: string[],
): unknown => {
	let value: unknown = updates;

	for (const key of path) {
		if (value === null || typeof value !== "object" || Array.isArray(value)) {
			return undefined;
		}

		value = (value as Record<string, unknown>)[key];
	}

	return value;
};

const latestObject = (updates: unknown): Record<string, unknown> | null => {
	if (updates === null || updates === undefined) {
		return null;
	}

	if (Array.isArray(updates)) {
		return latestObject(updates.at(-1));
	}

	if (typeof updates === "object") {
		return updates as Record<string, unknown>;
	}

	return null;
};

/*
compileFormat creates a formatter from a data-paint-format value.

Examples:

data-paint-format=".2f"
data-paint-format=".4f"
*/
const compileFormat = (format?: string): ((value: unknown) => string) => {
	const fixed = format?.match(/^\.(\d+)f$/);

	if (fixed) {
		const precision = Number(fixed[1]);

		return (value: unknown) =>
			typeof value === "number" ? value.toFixed(precision) : String(value);
	}

	return (value: unknown) => String(value);
};

/*
compileClasses creates a value-to-class map from data-paint-class.

Examples:

data-paint-class="true:active false:inactive"
data-paint-class="buy:text-green-500 sell:text-red-500"
data-paint-class="true:bg-green-500,text-white false:bg-red-500,text-white"
*/
const compileClasses = (value?: string): Map<string, string[]> => {
	const classes = new Map<string, string[]>();

	if (!value) return classes;

	for (const entry of value.split(/\s+/)) {
		const separator = entry.indexOf(":");

		if (separator === -1) continue;

		const key = entry.slice(0, separator);
		const classNames = entry
			.slice(separator + 1)
			.split(",")
			.filter(Boolean);

		if (!key || classNames.length === 0) continue;

		classes.set(key, classNames);
	}

	return classes;
};

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
export const Component = ({
	className,
	register,
	children,
}: ComponentProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const targets = useRef(new Map<string, PaintTarget>());

	useLayoutEffect(() => {
		if (!ref.current) return;

		for (const element of ref.current.querySelectorAll<HTMLElement>(
			"[data-paint]",
		)) {
			const key = element.dataset.paint;
			if (!key) continue;

			const classes = compileClasses(element.dataset.paintClass);
			const paintElement: PaintElement = {
				element,
				format: compileFormat(element.dataset.paintFormat),
				classes,
				classNames: [...new Set(Array.from(classes.values()).flat())],
			};

			const target = targets.current.get(key);

			if (target) {
				target.elements.push(paintElement);
				continue;
			}

			targets.current.set(key, {
				elements: [paintElement],
				path: key.split("."),
			});
		}

		const paint = (updates: unknown) => {
			if (!ref.current || targets.current.size === 0) return;

			const source = latestObject(updates);

			if (source === null) return;

			for (const target of targets.current.values()) {
				const value = getValue(source, target.path);

				if (value === undefined || value === null) continue;

				for (const {
					element,
					format,
					classes,
					classNames,
				} of target.elements) {
					element.textContent = format(value);

					if (classes.size === 0) continue;

					element.classList.remove(...classNames);

					const nextClasses = classes.get(String(value));

					if (nextClasses) {
						element.classList.add(...nextClasses);
					}
				}
			}
		};

		const unregister = register(paint);

		return () => {
			unregister?.();
			targets.current.clear();
		};
	}, [register]);

	return children({
		ref,
		className,
	});
};
