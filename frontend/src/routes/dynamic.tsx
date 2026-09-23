import { createFileRoute, Outlet } from "@tanstack/react-router";

/*
Everything under /dynamic is a graph being drawn, so this layer only makes room
for whichever one is: the index listing them, or one of them by name.
*/
export const Route = createFileRoute("/dynamic")({
	component: Outlet,
});
