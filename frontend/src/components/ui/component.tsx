import {
	type ReactNode,
	useCallback,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { getLastFrame, registerPainter } from "#/providers/ws-stores";
import {
	registerStreamCanvas,
	unregisterStreamCanvas,
} from "#/providers/stream-canvas";
import type { JSONSerializable, Paint } from "./paint";

/*
Component is a wrapper that takes care of boilerplate around the UI.
It is also able to switch from being React managed to being a static
HTML component that is updated via direct DOM manipulation.
This is useful for performance reasons when the component is rapid-fire
updated with real-time data, such as a chart or a table. In those cases,
React's diffing algorithm may introduce unnecessary overhead.

Array payloads do not enter React unless repeat is explicit. Most direct-paint
surfaces consume sparse arrays through data-scope bindings and must keep their
static labels mounted; only repeated rows need React to allocate slots.

Usage:

<Component
	className="metric-grid"
	registerKey="measurements"
	repeat
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

Rows are picked out of a batch with data-scope / data-filter, read as parallel
comma separated lists so a row can be pinned on more than one field at once:

	<div data-scope="source,symbol" data-filter="hawkes,BTC/USD">
		<span data-paint="metrics.spectral_radius.raw" data-paint-format=".3f" />
	</div>

data-paint-hold="2000" reads a flag over a window rather than instantaneously: a
true paints at once, a false only after the flag has stayed down that many
milliseconds. It is for latches that clear on the producer's own cycle — a
per-epoch readiness stamp — which strobe a status light if painted raw.

data-paint-format also takes "time", "date", "dir", and ".Nbp" for basis-point display
of return fractions, and the path segment "length" reads an array's size. data-paint-empty says how an empty string
reads. data-paint-absent says how a missing row reads, and belongs on bindings
whose wire key carries a complete map rather than a rolling batch — otherwise
every batch that happens to omit the row would blank a live reading.

A canvas plots a retained series from data-stream-value. An irregular process
adds the shape of its own samples:

	<canvas
		data-stream-filter="source=hawkes,symbol=BTC/USD"
		data-stream-id="at"
		data-stream-time="at"
		data-stream-value="metrics.conditional_intensity:buy.raw"
		data-stream-baseline="metrics.background_rate:buy.raw"
		data-stream-decay="metrics.excitation_decay:buy_from_buy.raw"
		data-stream-window="120"
		data-stream-rug=""
	/>

data-stream-time lays samples out at the instants they were observed instead of
spacing them evenly; data-stream-window bounds that history in seconds;
data-stream-decay relaxes the curve toward the baseline between observations at
the producer's own rate rather than joining them with a straight line; and
data-stream-rug ticks each observation so modelled stretches stay distinguishable
from measured ones.
*/
interface ComponentRenderProps {
	ref: React.RefObject<HTMLDivElement | null>;
	className?: string;
	slots: number[];
}

interface ComponentProps {
	className?: string;
	registerKey?: string;
	repeat?: boolean;
	select?: string;
	children: (props: ComponentRenderProps) => ReactNode;
}

type JSONRecord = { [key: string]: JSONSerializable | undefined };

type PaintDataset = DOMStringMap & {
	paint?: string;
	paintProp?: string;
	paintSuffix?: string;
	set?: string;
	setScale?: string;
	setDomain?: string;
	setThreshold?: string;
	append?: string;
	appendLimit?: string;
	appendWidth?: string;
	appendHeight?: string;
	streamFilter?: string;
	streamId?: string;
	streamValue?: string;
	streamValues?: string;
	streamLastId?: string;
	streamBaseline?: string;
	streamBaselineValue?: string;
	streamTime?: string;
	streamWindow?: string;
	streamDecay?: string;
	streamRug?: string;
	streamAppliedFilter?: string;
	target?: string;
	scope?: string;
	filter?: string;
	index?: string;
	paintFormat?: string;
	paintClass?: string;
	paintEmpty?: string;
	paintAbsent?: string;
	paintHold?: string;
	paintRaisedAt?: string;
};

type PaintBinding = {
	key: string;
	element: HTMLElement;
	dataset: PaintDataset;
	mode: "paint" | "set" | "append" | "stream";
	read: PathReader;
	write: TargetWriter;
	writePaintProperty: TargetWriter;
};

type PathReader = (updates: JSONSerializable) => JSONSerializable | undefined;

type TargetWriter = (element: HTMLElement, value: JSONSerializable) => void;

const pathReaders = new Map<string, PathReader>();
const targetWriters = new Map<string, TargetWriter>();
const listParts = new Map<string, string[]>();
const appendBuffers = new WeakMap<HTMLElement, Float64Array>();
const paintClassRules = new Map<
	string,
	Array<{ expected: string; tokens: string[] }>
>();

/*
COMPONENT_ROOT marks the element a Component hands its ref to, so a scan can
tell where one Component's surface ends and a nested one's begins.
*/
const COMPONENT_ROOT = "data-paint-root";

/*
inherited walks up from a binding for the row selector it belongs to, and stops
at the edge of its own Component.

Components nest: a kernel row scoped to one measurement source can hold a second
Component reading the readiness gates. The gate binding sits inside the row's
data-scope in the DOM, but it is not a row of that batch, and inheriting the
row's selector made it ask the readiness frame for a "source" field it does not
have — so every gate resolved to absent and every badge stayed on standby. A
selector only applies within the Component that declared it.
*/
const inherited = (
	root: HTMLElement,
	element: HTMLElement,
	selector: string,
): HTMLElement | undefined => {
	const ancestor = element.closest<HTMLElement>(selector);

	return ancestor !== null && (ancestor === root || root.contains(ancestor))
		? ancestor
		: undefined;
};

const scanTargets = (root: HTMLElement) => {
	const rootTargets = new Map<string, PaintBinding[]>();

	for (const element of root.querySelectorAll<HTMLElement>(
		"[data-paint], [data-set], [data-append], [data-stream-value]",
	)) {
		const key =
			element.dataset.paint ??
			element.dataset.set ??
			element.dataset.append ??
			element.dataset.streamValue;

		if (!key) {
			continue;
		}

		/*
			A binding belongs to the nearest Component above it. Without this an
			outer Component would also paint the bindings of any Component nested
			inside it, writing one key's frame into a surface bound to another's.
		*/
		if (element.closest(`[${COMPONENT_ROOT}]`) !== root) {
			continue;
		}

		const scopedParent = inherited(root, element, "[data-scope][data-filter]");
		const dataset: PaintDataset = {
			...element.dataset,
			scope: element.dataset.scope ?? scopedParent?.dataset.scope,
			filter: element.dataset.filter ?? scopedParent?.dataset.filter,
			index:
				element.dataset.index ??
				inherited(root, element, "[data-index]")?.dataset.index,
		};

		if (!rootTargets.has(key)) {
			rootTargets.set(key, []);
		}

		rootTargets.get(key)?.push({
			key,
			element,
			dataset,
			read: compilePath(key),
			write: compileTargetWriter(dataset.target),
			writePaintProperty: compileTargetWriter(dataset.paintProp),
			mode: element.dataset.streamValue
				? "stream"
				: element.dataset.append
					? "append"
					: element.dataset.set
						? "set"
						: "paint",
		});
	}

	return rootTargets;
};

const formatValue = (value: unknown, format: string | undefined): string => {
	/*
		A wall clock arrives as an RFC 3339 stamp. The terminal shows the engine's
		own time, so the stamp is read as an instant and printed in UTC rather than
		reformatted against whatever timezone the browser happens to sit in.
	*/
	if (format === "time" || format === "date") {
		const instant = new Date(String(value));

		if (!Number.isFinite(instant.getTime())) {
			throw new Error(
				`data-paint-format=${format} needs a timestamp: ${value}`,
			);
		}

		return format === "time"
			? instant.toISOString().slice(11, 19)
			: instant.toISOString().slice(0, 10);
	}

	if (format === "dir") {
		const lean = typeof value === "number" ? value : Number(value);

		if (!Number.isFinite(lean)) {
			throw new Error(`data-paint-format=dir needs a signed call: ${value}`);
		}

		if (lean > 0) {
			return "up";
		}

		if (lean < 0) {
			return "down";
		}

		return "flat";
	}

	/*
		Money arrives as a string, because a decimal that survived the wire
		without losing precision cannot be a float. A formatted field asks for
		a number, so a value that reads as one is treated as one — otherwise
		every price and balance would print at full stored precision.
	*/
	if (
		format &&
		typeof value === "string" &&
		value.trim() !== "" &&
		Number.isFinite(Number(value))
	) {
		value = Number(value);
	}

	if (format) {
		switch (typeof value) {
			case "number": {
				const showSign = format.startsWith("+");
				const numFmt = showSign ? format.slice(1) : format;

				if (!/^\.\d+(f|%|bp)?$/i.test(numFmt)) {
					throw new Error(`invalid data-paint-format: ${format}`);
				}

				const digits = Number.parseInt(numFmt.slice(1), 10);

				if (!Number.isInteger(digits)) {
					throw new Error(`invalid data-paint-format: ${format}`);
				}

				if (digits < 0 || digits > 100) {
					throw new Error(
						`data-paint-format fractional digits out of range: ${format}`,
					);
				}

				let formatted: string;

				if (numFmt.endsWith("%")) {
					formatted = `${(value * 100).toFixed(digits)}%`;
				} else if (numFmt.endsWith("bp")) {
					formatted = `${(value * 10000).toFixed(digits)}`;
				} else {
					formatted = value.toFixed(digits);
				}

				if (showSign && value >= 0) {
					return `+${formatted}`;
				}

				return formatted;
			}
		}
	}

	return String(value);
};

const readPath = (
	updates: JSONSerializable,
	path: string | undefined,
): JSONSerializable | undefined => compilePath(path)(updates);

const compilePath = (path: string | undefined): PathReader => {
	if (!path || path === "$") {
		return (updates) => updates;
	}

	const cached = pathReaders.get(path);

	if (cached !== undefined) {
		return cached;
	}

	const parts = path.split(".");
	const reader: PathReader = (updates) => {
		let value: JSONSerializable | undefined = updates;

		for (const part of parts) {
			if (value === undefined || value === null) {
				return undefined;
			}

			if (Array.isArray(value)) {
				if (part === "length") {
					value = value.length;
					continue;
				}

				const index = Number.parseInt(part, 10);

				if (!Number.isInteger(index)) {
					return undefined;
				}

				value = value[index];
				continue;
			}

			if (typeof value === "object" && !Array.isArray(value)) {
				value = (value as JSONRecord)[part];
				continue;
			}

			return undefined;
		}

		return value;
	};

	pathReaders.set(path, reader);
	return reader;
};

const splitList = (value: string): string[] => {
	const cached = listParts.get(value);

	if (cached !== undefined) {
		return cached;
	}

	const parts = value.split(",").map((entry) => entry.trim());
	listParts.set(value, parts);
	return parts;
};

/*
matchesScope answers whether one wire row is the row a binding asked for.
data-scope and data-filter are read as parallel lists, so a row can be pinned on
more than one field at once — a measurement belongs to both a source and a
symbol, and either alone selects the wrong row.
*/
const matchesScope = (
	item: JSONSerializable,
	scope: string,
	filter: string,
): boolean => {
	const scopes = splitList(scope);
	const filters = splitList(filter);

	if (scopes.length !== filters.length) {
		throw new Error(
			`data-scope and data-filter must pair up: ${scope} / ${filter}`,
		);
	}

	return scopes.every(
		(path, index) => String(readPath(item, path)) === filters[index],
	);
};

const selectScopedUpdates = (
	updates: JSONSerializable,
	dataset: PaintDataset,
): JSONSerializable | undefined => {
	/*
		A symbol-keyed map answers a scoped binding by lookup, and a miss is an
		answer: the engine has published nothing for that symbol. Falling back to
		the whole map would leave the previous symbol's reading on screen under the
		new symbol's name.
	*/
	if (
		!Array.isArray(updates) &&
		dataset.scope &&
		dataset.filter !== undefined &&
		typeof updates === "object" &&
		updates !== null
	) {
		const keyed = (updates as JSONRecord)[dataset.filter];

		return typeof keyed === "object" && keyed !== null && !Array.isArray(keyed)
			? keyed
			: undefined;
	}

	if (!Array.isArray(updates)) {
		return updates;
	}

	if (!dataset.scope || dataset.filter === undefined) {
		const index = Number.parseInt(dataset.index ?? "", 10);

		/*
			A binding that names no row is asking about the batch itself — its
			length, say. Paths that only make sense on a row read as absent against
			an array and are skipped, so nothing is invented here.
		*/
		if (!Number.isInteger(index)) {
			return updates;
		}

		return index >= 0 && index < updates.length ? updates[index] : undefined;
	}

	for (const item of updates) {
		if (item === null || typeof item !== "object" || Array.isArray(item)) {
			continue;
		}

		if (matchesScope(item as JSONSerializable, dataset.scope, dataset.filter)) {
			return item;
		}
	}

	return undefined;
};

const applyPaintClass = (
	element: HTMLElement,
	value: JSONSerializable,
): void => {
	const spec = element.dataset.paintClass;

	if (!spec) {
		return;
	}

	let rules = paintClassRules.get(spec);

	if (rules === undefined) {
		rules = compilePaintClasses(spec);
		paintClassRules.set(spec, rules);
	}

	for (const { expected, tokens } of rules) {
		for (const token of tokens) {
			if (!token) {
				continue;
			}

			element.classList.toggle(token, String(value) === expected);
		}
	}
};

const compilePaintClasses = (
	spec: string,
): Array<{ expected: string; tokens: string[] }> =>
	spec.split(/\s+/).flatMap((rule) => {
		const separator = rule.indexOf(":");

		if (separator === -1) {
			return [];
		}

		const expected = rule.slice(0, separator);
		const className = rule.slice(separator + 1);

		if (className === "") {
			return [];
		}

		const tokens: string[] = [];
		let depth = 0;
		let start = 0;

		for (let index = 0; index < className.length; index += 1) {
			const char = className[index];

			if (char === "[" || char === "(") {
				depth += 1;
			}

			if ((char === "]" || char === ")") && depth > 0) {
				depth -= 1;
			}

			if (char === "," && depth === 0) {
				tokens.push(className.slice(start, index));
				start = index + 1;
			}
		}

		tokens.push(className.slice(start));
		return [{ expected, tokens }];
	});

const compileTargetWriter = (target: string | undefined): TargetWriter => {
	if (!target) {
		return () => {};
	}

	const cached = targetWriters.get(target);

	if (cached !== undefined) {
		return cached;
	}

	if (target.startsWith("style.--")) {
		const property = target.slice("style.".length);
		const writer: TargetWriter = (element, value) => {
			const formatted = String(value);

			if (element.style.getPropertyValue(property) !== formatted) {
				element.style.setProperty(property, formatted);
			}
		};

		targetWriters.set(target, writer);
		return writer;
	}

	const parts = target.split(".");
	const property = parts.at(-1);
	const parents = parts.slice(0, -1);
	const writer: TargetWriter = (element, value) => {
		let current: unknown = element;

		for (const part of parents) {
			if (
				current === null ||
				current === undefined ||
				typeof current !== "object"
			) {
				return;
			}

			current = (current as Record<string, unknown>)[part];
		}

		if (
			!property ||
			current === null ||
			current === undefined ||
			typeof current !== "object"
		) {
			return;
		}

		const targetObject = current as Record<string, unknown>;

		if (targetObject[property] !== value) {
			targetObject[property] = value;
		}
	};

	targetWriters.set(target, writer);
	return writer;
};

const scaleSetValue = (
	value: JSONSerializable,
	dataset: PaintDataset,
	updates: JSONSerializable,
	scopedUpdates: JSONSerializable,
): JSONSerializable => {
	if (dataset.setScale === "sign-color") {
		const numericValue = Number(value);

		if (!Number.isFinite(numericValue)) {
			return "var(--f3)";
		}

		if (numericValue > 0) {
			return "var(--up)";
		}

		if (numericValue < 0) {
			return "var(--down)";
		}

		return "var(--f3)";
	}

	/*
		Health is a separate colour language from direction on purpose. A red
		"up/down" tone and a red "this is broken" tone in one panel are read as
		the same statement, so system state gets its own hue and leaves --up and
		--down to mean price and nothing else.
	*/
	if (dataset.setScale === "health-color") {
		const numericValue = Number(value);

		if (!Number.isFinite(numericValue)) {
			return "var(--f3)";
		}

		if (numericValue > 0) {
			return "var(--info)";
		}

		if (numericValue < 0) {
			return "var(--error)";
		}

		return "var(--warn)";
	}

	if (dataset.setScale === "bool-color") {
		return value === true ? "var(--up)" : "var(--line2)";
	}

	if (dataset.setScale === "activity-color") {
		if (value === "running") {
			return "var(--down)";
		}

		if (value === "done") {
			return "var(--up)";
		}

		return "var(--line2)";
	}

	if (dataset.setScale === "presence") {
		return true;
	}

	if (dataset.setScale === "presence-color") {
		return "var(--up)";
	}

	if (dataset.setScale === "above-threshold") {
		const numericValue = Number(value);

		if (!Number.isFinite(numericValue) || !dataset.setThreshold) {
			return "var(--f3)";
		}

		const threshold = Number(readPath(scopedUpdates, dataset.setThreshold));

		if (!Number.isFinite(threshold)) {
			return "var(--f3)";
		}

		if (numericValue > threshold) {
			return "var(--up)";
		}

		return "var(--down)";
	}

	if (dataset.setScale === "domain-percent") {
		const numericValue =
			typeof value === "number"
				? value
				: typeof value === "string" && value.trim() !== ""
					? Number(value)
					: Number.NaN;

		if (!Number.isFinite(numericValue) || !dataset.setDomain) {
			return "";
		}

		const values = splitList(dataset.setDomain).flatMap((path) => {
			const domainValue = readPath(scopedUpdates, path);

			if (typeof domainValue === "number" && Number.isFinite(domainValue)) {
				return [domainValue];
			}

			if (
				typeof domainValue === "string" &&
				domainValue.trim() !== "" &&
				Number.isFinite(Number(domainValue))
			) {
				return [Number(domainValue)];
			}

			return [];
		});

		if (values.length < 2) {
			return "";
		}

		const low = Math.min(...values);
		const high = Math.max(...values);

		if (!(high > low)) {
			return "50%";
		}

		const pad = Math.max((high - low) * 0.15, 1e-6);
		const domainLow = low - pad;
		const domainHigh = high + pad;
		const percent =
			((numericValue - domainLow) / (domainHigh - domainLow)) * 100;

		return `${Math.min(100, Math.max(0, percent)).toFixed(3)}%`;
	}

	if (dataset.setScale !== "max-abs") {
		return value;
	}

	const numericValue = Number(value);

	if (!Number.isFinite(numericValue) || !Array.isArray(updates)) {
		return value;
	}

	let denominator = 0;

	for (const entry of updates) {
		const numericEntry = Number(entry);

		if (!Number.isFinite(numericEntry)) {
			continue;
		}

		denominator = Math.max(denominator, Math.abs(numericEntry));
	}

	if (denominator <= 0) {
		return 0;
	}

	return numericValue / denominator;
};

const appendTargetValue = (
	element: HTMLElement,
	dataset: PaintDataset,
	value: JSONSerializable,
): void => {
	const numericValue = Number(value);

	if (!Number.isFinite(numericValue)) {
		return;
	}

	const limit = Number.parseInt(dataset.appendLimit ?? "40", 10);
	const width = Number.parseFloat(dataset.appendWidth ?? "150");
	const height = Number.parseFloat(dataset.appendHeight ?? "30");

	if (
		!Number.isInteger(limit) ||
		limit < 2 ||
		!Number.isFinite(width) ||
		!Number.isFinite(height)
	) {
		return;
	}

	let values = appendBuffers.get(element);

	if (values === undefined || values.length !== limit) {
		values = new Float64Array(limit);
		values.fill(Number.NaN);
		appendBuffers.set(element, values);
	}

	values.copyWithin(0, 1);
	values[values.length - 1] = numericValue;
	const retained = Array.from(values).filter(Number.isFinite);

	const points = retained
		.map((entry, index) => {
			const denominator = Math.max(retained.length - 1, 1);
			const x = (index / denominator) * width;
			const clamped = Math.min(Math.max(entry, 0), 1);
			const y = height - 1 - clamped * (height - 4);

			return `${x.toFixed(1)},${y.toFixed(1)}`;
		})
		.join(" ");

	element.setAttribute(
		dataset.target ?? "points",
		element.tagName === "polygon"
			? `${points} ${width.toFixed(1)},${height.toFixed(1)} 0.0,${height.toFixed(1)}`
			: points,
	);
};

const isRaised = (value: JSONSerializable): boolean =>
	value === true || value === "true";

/*
holdSuppresses answers whether this paint should be skipped because the flag it
carries has not been down long enough to believe. Returns false for every
binding that did not ask for a hold, which is all of them by default.
*/
const holdSuppresses = (
	element: HTMLElement,
	dataset: PaintDataset,
	value: JSONSerializable,
): boolean => {
	const window = Number(dataset.paintHold);

	if (!Number.isFinite(window) || window <= 0) {
		return false;
	}

	const now = performance.now();

	if (isRaised(value)) {
		element.dataset.paintRaisedAt = String(now);
		return false;
	}

	const raisedAt = Number(element.dataset.paintRaisedAt);

	return Number.isFinite(raisedAt) && now - raisedAt < window;
};

const updateTargets = (
	targets: Map<string, PaintBinding[]>,
	updates: JSONSerializable,
) => {
	if (updates === undefined || updates === null) {
		return;
	}

	for (const targetsByKey of targets.values()) {
		for (const target of targetsByKey) {
			if (target.mode === "stream") {
				continue;
			}

			const scopedUpdates = selectScopedUpdates(updates, target.dataset);

			if (scopedUpdates === undefined || scopedUpdates === null) {
				/*
					Direct paint retains by nature, which is what keeps a surface alive
					between the sparse batches a rolling kernel publishes. Where the batch
					is instead a complete map — every symbol the engine currently holds —
					a miss is a real answer, and data-paint-absent says how it reads.
				*/
				if (
					target.mode === "paint" &&
					target.dataset.paintAbsent !== undefined
				) {
					if (target.element.textContent !== target.dataset.paintAbsent) {
						target.element.textContent = target.dataset.paintAbsent;
					}
				}

				continue;
			}

			const value = target.read(scopedUpdates);

			if (value === undefined || value === null) {
				continue;
			}

			/*
				Some flags are latches that clear on the producer's own cycle rather
				than states that persist: a per-epoch readiness stamp is true from the
				moment its stage reports until the next epoch resets it. Painted
				instantaneously, a status light bound to one strobes.

				data-paint-hold reads such a flag over a window instead: a true is
				taken immediately, and a false is only taken once the flag has stayed
				false for the whole window. Nothing is invented — the surface still
				goes false when the producer genuinely stops raising it, just not on
				the gap between two raises.
			*/
			if (holdSuppresses(target.element, target.dataset, value)) {
				continue;
			}

			if (target.mode === "set") {
				target.write(
					target.element,
					scaleSetValue(value, target.dataset, updates, scopedUpdates),
				);
				continue;
			}

			if (target.mode === "append") {
				appendTargetValue(target.element, target.dataset, value);
				continue;
			}

			/*
				An empty string is a real answer — the model ran and named nothing.
				data-paint-empty says how that reads, so the slot states the absence
				instead of looking like a panel that never received data.
			*/
			const formatted =
				value === "" && target.dataset.paintEmpty !== undefined
					? target.dataset.paintEmpty
					: `${formatValue(value, target.dataset.paintFormat)}${target.dataset.paintSuffix ?? ""}`;

			if (target.dataset.paintProp) {
				target.writePaintProperty(target.element, formatted);
			} else if (target.element.textContent !== formatted) {
				target.element.textContent = formatted;
			}

			applyPaintClass(target.element, value);
		}
	}
};

export const Component = ({
	className,
	registerKey,
	repeat = false,
	select,
	children,
}: ComponentProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const targets = useRef<Map<string, PaintBinding[]>>(new Map());
	const latest = useRef<JSONSerializable | undefined>(undefined);
	const [slots, setSlots] = useState<number[]>([]);

	const repaint = useCallback(() => {
		if (latest.current === undefined) {
			return;
		}

		const updates = select ? readPath(latest.current, select) : latest.current;

		if (updates === undefined || updates === null) {
			return;
		}

		updateTargets(targets.current, updates);
	}, [select]);

	useLayoutEffect(() => {
		if (!ref.current) {
			return;
		}

		/*
			The marker is stamped rather than passed through the render props: call
			sites hand the ref to whatever element suits them, and a surface should
			not have to remember to spread an extra attribute for nesting to work.
		*/
		ref.current.setAttribute(COMPONENT_ROOT, "");
		targets.current = scanTargets(ref.current);

		for (const bindings of targets.current.values()) {
			for (const binding of bindings) {
				if (
					binding.mode === "stream" &&
					binding.element instanceof HTMLCanvasElement
				) {
					registerStreamCanvas(binding.element, binding.dataset);
				}
			}
		}

		repaint();
	});

	useLayoutEffect(() => {
		const root = ref.current;

		return () => {
			for (const canvas of root?.querySelectorAll<HTMLCanvasElement>(
				"canvas[data-stream-value]",
			) ?? []) {
				unregisterStreamCanvas(canvas);
			}
		};
	}, []);

	/*
		A canvas sizes itself from the box it was painted into. Nothing republishes
		when the window changes, so without this a resized chart keeps drawing at
		the geometry it last saw.
	*/
	useLayoutEffect(() => {
		const root = ref.current;

		if (root === null) {
			return;
		}

		const observer = new ResizeObserver(repaint);

		observer.observe(root);

		return () => observer.disconnect();
	}, [repaint]);

	useLayoutEffect(() => {
		if (!registerKey) {
			return;
		}

		const paint: Paint = (updates) => {
			latest.current = updates;

			if (updates === undefined || updates === null) {
				return;
			}

			if (select) {
				const selectedUpdates = readPath(updates, select);

				if (selectedUpdates === undefined || selectedUpdates === null) {
					return;
				}

				updates = selectedUpdates;
			}

			if (repeat && Array.isArray(updates)) {
				setSlots((current) => {
					if (current.length === updates.length) {
						return current;
					}

					return Array.from({ length: updates.length }, (_, index) => index);
				});
			}

			updateTargets(targets.current, updates);
		};

		const unregister = registerPainter(registerKey, paint);

		/*
			A fresh mount has an empty `latest` ref even when the registerKey
			already has data flowing — routing tears the previous instance's
			ref down along with its DOM. Replaying the retained last frame here
			is what makes revisiting a page show its data immediately instead
			of sitting blank until the next websocket tick.
		*/
		const seed = latest.current ?? getLastFrame(registerKey);

		if (seed !== undefined) {
			paint(seed);
		}

		return () => {
			unregister?.();
		};
	}, [registerKey, repeat, select]);

	return children({
		ref,
		className,
		slots,
	});
};
