"use client";

import { type ReactNode, useEffect, useSyncExternalStore } from "react";
import { cn } from "@/lib/utils";
import { Alert } from "./alert";

/*
Toast reports something that happened, to a surface that was not asking.

Alert is the band that says the region under it is not telling the whole truth;
a toast says an action you took succeeded or failed, and then leaves. The
difference is who is waiting: an Alert is part of the layout, a toast interrupts
nothing and is gone before it would have to be laid out around.

The manager is a plain subscribable rather than this project's store, because
everything in this directory is meant to be copied into another project whose
only obligation is `cn` — see the library's README. React's own
useSyncExternalStore is enough to read it.
*/

export type ToastTone = "error" | "info" | "success" | "warning";

export type Toast = {
	id: string;
	title?: ReactNode;
	description?: ReactNode;
	type?: ToastTone;
	/* Milliseconds to stay up. Zero keeps it until it is dismissed. */
	timeout?: number;
};

export type ToastRequest = Omit<Toast, "id"> & { id?: string };

const createToastManager = () => {
	let toasts: Toast[] = [];
	const listeners = new Set<() => void>();

	const emit = () => {
		for (const listener of listeners) {
			listener();
		}
	};

	return {
		subscribe(listener: () => void) {
			listeners.add(listener);

			return () => {
				listeners.delete(listener);
			};
		},

		read(): Toast[] {
			return toasts;
		},

		add(request: ToastRequest): string {
			const id =
				request.id ??
				`toast-${Date.now()}-${Math.random().toString(36).slice(2)}`;

			toasts = [...toasts, { ...request, id }];
			emit();

			return id;
		},

		dismiss(id: string) {
			const remaining = toasts.filter((toast) => toast.id !== id);

			if (remaining.length === toasts.length) {
				return;
			}

			toasts = remaining;
			emit();
		},

		clear() {
			if (toasts.length === 0) {
				return;
			}

			toasts = [];
			emit();
		},
	};
};

export const toastManager = createToastManager();

const empty: Toast[] = [];

/*
Toasts renders whatever the manager is holding. It is mounted once by whatever
surface can raise one; nothing subscribes to it that does not display it.
*/
export const Toasts = ({ className }: { className?: string }) => {
	const toasts = useSyncExternalStore(
		toastManager.subscribe,
		toastManager.read,
		() => empty,
	);

	if (toasts.length === 0) {
		return null;
	}

	return (
		<div
			className={cn(
				"pointer-events-none fixed right-3 bottom-3 z-[9999] flex w-80 flex-col gap-2",
				className,
			)}
		>
			{toasts.map((toast) => (
				<ToastRow key={toast.id} toast={toast} />
			))}
		</div>
	);
};

const ToastRow = ({ toast }: { toast: Toast }) => {
	const { id, timeout } = toast;

	useEffect(() => {
		if (!timeout) {
			return;
		}

		const handle = window.setTimeout(() => toastManager.dismiss(id), timeout);

		return () => window.clearTimeout(handle);
	}, [id, timeout]);

	return (
		<button
			type="button"
			onClick={() => toastManager.dismiss(id)}
			className="pointer-events-auto text-left"
		>
			<Alert variant={toast.type ?? "info"}>
				<span className="flex min-w-0 flex-col gap-0.5">
					{toast.title ? (
						<span className="font-medium">{toast.title}</span>
					) : null}
					{toast.description ? (
						<span className="text-(--f3)">{toast.description}</span>
					) : null}
				</span>
			</Alert>
		</button>
	);
};
