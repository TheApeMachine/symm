import { createContext, type ReactNode, useContext } from "react";

/* The router supplies one active surface; its placement belongs to the authored shell graph. */
export const SurfaceOutletContext = createContext<ReactNode>(null);
export const SurfaceOutlet = () => <>{useContext(SurfaceOutletContext)}</>;
