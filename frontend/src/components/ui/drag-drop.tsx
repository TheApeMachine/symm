"use client";

import {
	createContext,
	type DragEvent,
	type ReactNode,
	useCallback,
	useContext,
	useMemo,
	useRef,
} from "react";

/*
DragDrop is a tiny, payload-generic drag-and-drop primitive. A provider owns
a single in-flight payload ref so Draggable and DropTarget can communicate
without serializing through the browser's dataTransfer (which is read-only
during dragover and forbids structured data). View-transition morphing on
drop is opt-in via the `morph` flag on DropTarget.

Generic over the payload shape so consumers (the dashboard grid, a kanban
board, anything else) define their own discriminated union.
*/

interface DragDropContextValue<TPayload> {
	begin: (payload: TPayload) => void;
	end: () => void;
	read: () => TPayload | null;
}

const DragDropContext = createContext<DragDropContextValue<unknown> | null>(
	null,
);

interface DragDropProviderProps {
	children: ReactNode;
}

export function DragDropProvider({ children }: DragDropProviderProps) {
	const payloadRef = useRef<unknown>(null);

	const value = useMemo<DragDropContextValue<unknown>>(
		() => ({
			begin: (payload) => {
				payloadRef.current = payload;
			},
			end: () => {
				payloadRef.current = null;
			},
			read: () => payloadRef.current,
		}),
		[],
	);

	return (
		<DragDropContext.Provider value={value}>
			{children}
		</DragDropContext.Provider>
	);
}

export function useDragDrop<TPayload>(): DragDropContextValue<TPayload> {
	const value = useContext(DragDropContext);
	if (!value)
		throw new Error("useDragDrop must be used inside <DragDropProvider>");
	return value as DragDropContextValue<TPayload>;
}

const morphIfSupported = (mutate: () => void) => {
	const doc = document as Document & {
		startViewTransition?: (cb: () => void) => unknown;
	};

	if (typeof doc.startViewTransition === "function") {
		doc.startViewTransition(mutate);
		return;
	}

	mutate();
};

interface DraggableProps<TPayload> {
	payload: TPayload;
	children: ReactNode;
	className?: string;
	ariaLabel?: string;
	as?: "div" | "button" | "article" | "section";
	disabled?: (target: EventTarget | null) => boolean;
	style?: React.CSSProperties;
}

export function Draggable<TPayload>({
	payload,
	children,
	className,
	ariaLabel,
	as = "div",
	disabled,
	style,
}: DraggableProps<TPayload>) {
	const dnd = useDragDrop<TPayload>();

	const onDragStart = useCallback(
		(event: DragEvent<HTMLElement>) => {
			if (disabled?.(event.target)) return;

			event.dataTransfer.effectAllowed = "copyMove";
			event.dataTransfer.setData("text/plain", "");
			dnd.begin(payload);
			event.currentTarget.classList.add("dash-dragging");
		},
		[dnd, payload, disabled],
	);

	const onDragEnd = useCallback((event: DragEvent<HTMLElement>) => {
		event.currentTarget.classList.remove("dash-dragging");
	}, []);

	const Tag = as;

	return (
		<Tag
			draggable
			data-dash-drag
			aria-label={ariaLabel}
			onDragStart={onDragStart}
			onDragEnd={onDragEnd}
			className={className}
			style={style}
			type={as === "button" ? "button" : undefined}
		>
			{children}
		</Tag>
	);
}

interface DropTargetProps<TPayload> {
	onDrop: (payload: TPayload) => void;
	onEnter?: (payload: TPayload) => void;
	morph?: boolean;
	children: ReactNode;
	className?: string;
	ariaLabel?: string;
	as?: "div" | "section" | "article";
	style?: React.CSSProperties;
	highlightOnHover?: boolean;
}

export function DropTarget<TPayload>({
	onDrop,
	onEnter,
	morph = false,
	children,
	className,
	ariaLabel,
	as = "div",
	style,
	highlightOnHover = true,
}: DropTargetProps<TPayload>) {
	const dnd = useDragDrop<TPayload>();

	const onDragOver = useCallback((event: DragEvent<HTMLElement>) => {
		event.preventDefault();
		event.dataTransfer.dropEffect = "move";
	}, []);

	const onDragEnter = useCallback(
		(event: DragEvent<HTMLElement>) => {
			event.preventDefault();
			const payload = dnd.read();
			if (payload !== null) onEnter?.(payload);
			if (highlightOnHover) event.currentTarget.classList.add("dash-over");
		},
		[dnd, onEnter, highlightOnHover],
	);

	const onDragLeave = useCallback((event: DragEvent<HTMLElement>) => {
		event.currentTarget.classList.remove("dash-over");
	}, []);

	const handleDrop = useCallback(
		(event: DragEvent<HTMLElement>) => {
			event.preventDefault();
			event.stopPropagation();
			event.currentTarget.classList.remove("dash-over");

			const payload = dnd.read();
			dnd.end();
			if (payload === null) return;

			if (morph) morphIfSupported(() => onDrop(payload));
			else onDrop(payload);
		},
		[dnd, morph, onDrop],
	);

	const Tag = as;

	return (
		<Tag
			data-dash-drag
			aria-label={ariaLabel}
			onDragOver={onDragOver}
			onDragEnter={onDragEnter}
			onDragLeave={onDragLeave}
			onDrop={handleDrop}
			className={className}
			style={style}
		>
			{children}
		</Tag>
	);
}
