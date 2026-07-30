import { type ReactNode, useLayoutEffect, useRef, useState } from "react";

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
	target: string;
};

type PaintScope = {
	element: HTMLElement;
	index: number;
	key?: string;
	targets: Map<string, PaintTarget>;
};

type StreamTarget = {
	element: HTMLCanvasElement;
	filter: string[];
	key: string[];
	value: string[];
};

const applyTarget = (
	element: HTMLElement,
	target: string,
	formatted: string,
) => {
	if (target === "text") {
		element.textContent = formatted;
		return;
	}

	if (target.startsWith("style.")) {
		element.style.setProperty(
			target
				.slice("style.".length)
				.replace(/[A-Z]/g, (match) => `-${match.toLowerCase()}`),
			formatted,
		);
		return;
	}

	if (target.startsWith("attr.")) {
		element.setAttribute(target.slice("attr.".length), formatted);
		return;
	}

	if (target in element) {
		Reflect.set(element, target, formatted);
		return;
	}

	element.setAttribute(target, formatted);
};

const streamValue = (
	row: Record<string, unknown>,
	path: string[],
): number | null => {
	const value = getValue(row, path);

	return typeof value === "number" && Number.isFinite(value) ? value : null;
};

const streamMatches = (
	row: Record<string, unknown>,
	filters: string[],
): boolean =>
	filters.every((filter) => {
		const separator = filter.indexOf("=");
		if (separator === -1) return false;

		return (
			String(getValue(row, filter.slice(0, separator).split("."))) ===
			filter.slice(separator + 1)
		);
	});

const paintStream = (stream: StreamTarget, row: Record<string, unknown>) => {
	if (!streamMatches(row, stream.filter)) return;

	const value = streamValue(row, stream.value);
	if (value === null) return;

	const identity = stream.key
		.map((path) => getValue(row, path.split(".")))
		.join("\u0000");
	if (identity === stream.element.dataset.streamLast) return;

	const canvas = stream.element;
	const width = Math.max(1, canvas.clientWidth);
	const height = Math.max(1, canvas.clientHeight);
	const ratio = window.devicePixelRatio || 1;

	if (
		canvas.width !== Math.floor(width * ratio) ||
		canvas.height !== Math.floor(height * ratio)
	) {
		canvas.width = Math.floor(width * ratio);
		canvas.height = Math.floor(height * ratio);
		delete canvas.dataset.streamY;
		delete canvas.dataset.streamScale;
	}

	const context = canvas.getContext("2d");
	if (context === null) return;

	context.setTransform(ratio, 0, 0, ratio, 0, 0);
	const scale = Number(canvas.dataset.streamScale ?? value);
	if (!Number.isFinite(scale) || scale <= 0) return;

	canvas.dataset.streamScale = String(scale);
	canvas.dataset.streamLast = identity;
	const y = height - Math.min(height, (value / scale) * height);
	const previousY = Number(canvas.dataset.streamY ?? y);

	context.drawImage(canvas, -1, 0, width, height);
	context.clearRect(width - 1, 0, 1, height);
	context.strokeStyle = getComputedStyle(canvas)
		.getPropertyValue("--acc")
		.trim();
	context.lineWidth = 1.5;
	context.beginPath();
	context.moveTo(width - 2, previousY);
	context.lineTo(width - 1, y);
	context.stroke();

	canvas.dataset.streamY = String(y);
};

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

export const Component = ({
	className,
	register,
	select,
	children,
}: ComponentProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const latest = useRef<unknown>(undefined);
	const [slots, setSlots] = useState<number[]>([]);

	useLayoutEffect(() => {
		if (!ref.current) return;

		const extractTargetsFromScope = (
			scopeElement: HTMLElement,
		): Map<string, PaintTarget> => {
			const targetMap = new Map<string, PaintTarget>();

			for (const element of scopeElement.querySelectorAll<HTMLElement>(
				"[data-paint], [data-set]",
			)) {
				const key = element.dataset.paint ?? element.dataset.set;
				if (!key) continue;

				const classes = element.dataset.paint
					? compileClasses(element.dataset.paintClass)
					: new Map<string, string[]>();
				const paintElement: PaintElement = {
					element,
					format: compileFormat(element.dataset.paintFormat),
					classes,
					classNames: [...new Set(Array.from(classes.values()).flat())],
					target: element.dataset.target ?? element.dataset.paintProp ?? "text",
				};

				const existingTarget = targetMap.get(key);
				if (existingTarget) {
					existingTarget.elements.push(paintElement);
				} else {
					targetMap.set(key, {
						elements: [paintElement],
						path: key.split("."),
					});
				}
			}

			return targetMap;
		};

		const scopes = Array.from(
			ref.current.querySelectorAll<HTMLElement>("[data-index]"),
		).map((element): PaintScope => {
			const index = Number(element.dataset.index);
			if (!Number.isInteger(index) || index < 0) {
				throw new Error(
					"Component: data-index requires a non-negative integer",
				);
			}

			return {
				element,
				index,
				key: element.dataset.key,
				targets: extractTargetsFromScope(element),
			};
		});

		const streams = Array.from(
			ref.current.querySelectorAll<HTMLCanvasElement>(
				"canvas[data-stream-value]",
			),
		).map(
			(element): StreamTarget => ({
				element,
				filter:
					element.dataset.streamFilter?.split(/\s+/).filter(Boolean) ?? [],
				key: element.dataset.streamId?.split("+").filter(Boolean) ?? [],
				value: element.dataset.streamValue?.split(".") ?? [],
			}),
		);

		const rootTargets = new Map<string, PaintTarget>();
		for (const element of ref.current.querySelectorAll<HTMLElement>(
			"[data-paint], [data-set]",
		)) {
			if (element.closest("[data-index]")) {
				continue;
			}

			const key = element.dataset.paint ?? element.dataset.set;
			if (!key) continue;

			const classes = element.dataset.paint
				? compileClasses(element.dataset.paintClass)
				: new Map<string, string[]>();
			const paintElement: PaintElement = {
				element,
				format: compileFormat(element.dataset.paintFormat),
				classes,
				classNames: [...new Set(Array.from(classes.values()).flat())],
				target: element.dataset.target ?? element.dataset.paintProp ?? "text",
			};

			const existingTarget = rootTargets.get(key);
			if (existingTarget) {
				existingTarget.elements.push(paintElement);
			} else {
				rootTargets.set(key, {
					elements: [paintElement],
					path: key.split("."),
				});
			}
		}

		const paintTargets = (
			targets: Map<string, PaintTarget>,
			source: Record<string, unknown>,
		) => {
			for (const paintTarget of targets.values()) {
				const value = getValue(source, paintTarget.path);
				if (value === undefined || value === null) continue;

				for (const {
					element,
					format,
					classes,
					classNames,
					target,
				} of paintTarget.elements) {
					const formatted = format(value);
					applyTarget(element, target, formatted);

					if (classes.size === 0) continue;

					element.classList.remove(...classNames);
					const nextClasses = classes.get(String(value));
					if (nextClasses) element.classList.add(...nextClasses);
				}
			}
		};

		const paint = (updates: unknown) => {
			latest.current = updates;
			const source = latestObject(updates);
			if (source === null) return;

			if (select) {
				const path = select === "$" ? [] : select.split(".");
				const direct = getValue(source, path);
				const selected = Array.isArray(updates)
					? updates.map((row) => getValue(row as Record<string, unknown>, path))
					: Array.isArray(direct)
						? direct
						: Object.values(source)
								.map((row) => getValue(row as Record<string, unknown>, path))
								.filter((row) => row !== undefined);

				const nextRows = selected.filter(
					(row): row is Record<string, unknown> =>
						row !== null && typeof row === "object" && !Array.isArray(row),
				);
				const key = scopes[0]?.key;

				for (const stream of streams) {
					for (const row of nextRows) paintStream(stream, row);
				}

				if (scopes.length === 0 && streams.length > 0) {
					paintTargets(rootTargets, source);
					return;
				}

				if (key) {
					const existing = new Map(
						scopes.flatMap((scope) => {
							const value = scope.element.dataset.keyValue;
							return value === undefined ? [] : [[value, scope] as const];
						}),
					);
					const empty = scopes.filter(
						(scope) => scope.element.dataset.keyValue === undefined,
					);
					let additions = 0;

					for (const row of nextRows) {
						const value = getValue(row, key.split("."));
						if (value === undefined || value === null) continue;

						const identity = String(value);
						let scope = existing.get(identity);
						if (scope === undefined) {
							scope = empty.shift();
						}

						if (scope === undefined) {
							additions += 1;
							continue;
						}

						scope.element.dataset.keyValue = identity;
						existing.set(identity, scope);
						paintTargets(scope.targets, row);
					}

					if (additions > 0) {
						setSlots(
							Array.from(
								{ length: slots.length + additions },
								(_, index) => index,
							),
						);
					}
				} else if (slots.length < nextRows.length) {
					setSlots(
						Array.from({ length: nextRows.length }, (_, index) => index),
					);

					return;
				}

				paintTargets(rootTargets, source);

				if (!key) {
					for (const scope of scopes) {
						const row = nextRows[scope.index];
						if (row) paintTargets(scope.targets, row);
					}
				}

				return;
			}

			paintTargets(rootTargets, source);
		};

		const unregister = register(paint);
		if (latest.current !== undefined) paint(latest.current);

		return () => {
			unregister?.();
		};
	}, [register, select, slots]);

	return children({
		ref,
		className,
		slots,
	});
};
