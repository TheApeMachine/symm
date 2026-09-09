import { toastManager } from "#/components/ui/toast";
import type { ToastTypes } from "./types";

export enum ToastActionTypes {
	ADD_TOAST = "ADD_TOAST",
	REMOVE_TOAST = "REMOVE_TOAST",
	SET_HEIGHT = "SET_HEIGHT",
	SET_EXITING = "SET_EXITING",
}

export type ToastAction =
	| {
			type: ToastActionTypes.ADD_TOAST;
			title: string;
			message: string;
			toastType?: ToastTypes;
			duration?: number;
	  }
	| {
			type: ToastActionTypes.REMOVE_TOAST;
			id: string;
	  }
	| {
			type: ToastActionTypes.SET_HEIGHT;
			id: string;
			height: number;
	  }
	| {
			type: ToastActionTypes.SET_EXITING;
			id: string;
	  };

function mapFlumeToastType(
	toastType?: ToastTypes,
): "error" | "info" | "success" | "warning" | undefined {
	if (toastType === "danger") return "error";
	if (toastType === undefined) return "info";
	return toastType;
}

/** Show a flume graph toast using the app UI toast system (`#/components/ui/toast`). */
export function dispatchFlumeToastAction(action: ToastAction): void {
	if (action.type !== ToastActionTypes.ADD_TOAST) {
		return;
	}
	toastManager.add({
		title: action.title,
		description: action.message,
		type: mapFlumeToastType(action.toastType),
		timeout: action.duration ?? 10_000,
	});
}
