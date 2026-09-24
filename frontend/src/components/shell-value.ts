import { useSelector } from "@tanstack/react-store";
import { boundAtom } from "#/collections/app";

/*
useShellValue reads a readout of the application chrome from the running graph:
the value last bound to component's value port in the ui_shell graph. Absent
until the graph has said something, never a default.
*/
export const useShellValue = (component: string): number | undefined =>
	useSelector(boundAtom, (graphs) => {
		const value = graphs.ui_shell?.[component]?.value;
		return typeof value === "number" ? value : undefined;
	});
