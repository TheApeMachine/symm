export type ColorMode = "light" | "dim" | "dark" | "system";

export type ResolvedColorMode = Exclude<ColorMode, "system">;

export type VisualTheme =
	| "default"
	| "neumorphic"
	| "glassmorphic"
	| "neo-brutalism"
	| "claymorphism"
	| "blueprint"
	| "aurora";

export const STORAGE_KEY_MODE = "caramba.theme";

export const STORAGE_KEY_CONTRAST = "caramba.contrast";

export const STORAGE_KEY_VISUAL_THEME = "caramba.visual-theme";

export const MODE_CLASSES: ResolvedColorMode[] = ["light", "dim", "dark"];

export const VISUAL_THEME_MARKERS: Record<VisualTheme, string | null> = {
	default: null,
	neumorphic: "theme-neumorphic",
	glassmorphic: "theme-glassmorphic",
	"neo-brutalism": "theme-neo-brutalism",
	claymorphism: "theme-claymorphism",
	blueprint: "theme-blueprint",
	aurora: "theme-aurora",
};

export const VISUAL_THEME_MARKER_CLASSES = Object.values(
	VISUAL_THEME_MARKERS,
).filter((marker): marker is string => marker !== null);

export const VISUAL_THEME_STYLESHEET_ID = "caramba-visual-theme";

export const VISUAL_THEME_HREFS: Record<VisualTheme, string | null> = {
	default: null,
	neumorphic: "/themes/neumorphic.css",
	glassmorphic: "/themes/glassmorphic.css",
	"neo-brutalism": "/themes/neo-brutalism.css",
	claymorphism: "/themes/claymorphism.css",
	blueprint: "/themes/blueprint.css",
	aurora: "/themes/aurora.css",
};

export const VISUAL_THEME_OPTIONS: VisualTheme[] = [
	"default",
	"neumorphic",
	"glassmorphic",
	"neo-brutalism",
	"claymorphism",
	"blueprint",
	"aurora",
];

export const isColorMode = (value: string | null): value is ColorMode =>
	value === "light" ||
	value === "dim" ||
	value === "dark" ||
	value === "system";

export const isVisualTheme = (value: string | null): value is VisualTheme =>
	VISUAL_THEME_OPTIONS.includes(value as VisualTheme);

export const resolveSystemColorMode = (): ResolvedColorMode => {
	if (typeof window === "undefined") return "dark";
	return window.matchMedia("(prefers-color-scheme: dark)").matches
		? "dark"
		: "light";
};

export const resolveColorMode = (mode: ColorMode): ResolvedColorMode =>
	mode === "system" ? resolveSystemColorMode() : mode;

export const applyColorMode = (resolvedMode: ResolvedColorMode): void => {
	const root = document.documentElement;
	for (const modeClass of MODE_CLASSES) {
		root.classList.toggle(modeClass, modeClass === resolvedMode);
	}
};

export const applyContrast = (enabled: boolean): void => {
	document.documentElement.classList.toggle("contrast", enabled);
};

export const applyVisualThemeMarkers = (visualTheme: VisualTheme): void => {
	const root = document.documentElement;
	for (const markerClass of VISUAL_THEME_MARKER_CLASSES) {
		root.classList.remove(markerClass);
	}
	const activeMarker = VISUAL_THEME_MARKERS[visualTheme];
	if (activeMarker !== null) {
		root.classList.add(activeMarker);
	}
};

export const applyVisualTheme = (visualTheme: VisualTheme): void => {
	const root = document.documentElement;
	root.dataset.visualTheme = visualTheme;
	applyVisualThemeMarkers(visualTheme);

	const href = VISUAL_THEME_HREFS[visualTheme];
	const existing = document.getElementById(
		VISUAL_THEME_STYLESHEET_ID,
	) as HTMLLinkElement | null;

	if (href === null) {
		existing?.remove();
		return;
	}

	const absoluteHref = new URL(href, window.location.origin).href;

	if (existing) {
		if (existing.href !== absoluteHref) existing.href = href;
		document.head.appendChild(existing);
		return;
	}

	const link = document.createElement("link");
	link.id = VISUAL_THEME_STYLESHEET_ID;
	link.rel = "stylesheet";
	link.href = href;
	document.head.appendChild(link);
};

export const applyAppearance = ({
	mode,
	contrast,
	visualTheme,
}: {
	mode: ColorMode;
	contrast: boolean;
	visualTheme: VisualTheme;
}): void => {
	applyColorMode(resolveColorMode(mode));
	applyContrast(contrast);
	applyVisualTheme(visualTheme);
};

export const readBootstrappedVisualTheme = (): VisualTheme => {
	if (typeof document === "undefined") return "default";
	const fromDocument = document.documentElement.dataset.visualTheme;
	if (isVisualTheme(fromDocument ?? null)) return fromDocument as VisualTheme;
	return readStoredVisualTheme();
};

export const readStoredVisualTheme = (): VisualTheme => {
	if (typeof window === "undefined") return "default";
	const stored = window.localStorage.getItem(STORAGE_KEY_VISUAL_THEME);
	if (isVisualTheme(stored)) return stored;
	return "default";
};

export const readBootstrappedContrast = (): boolean => {
	if (typeof document === "undefined") return false;
	return document.documentElement.classList.contains("contrast");
};

export const readStoredContrast = (): boolean => {
	if (typeof window === "undefined") return false;
	return window.localStorage.getItem(STORAGE_KEY_CONTRAST) === "1";
};

export const readBootstrappedMode = (): ColorMode => {
	if (typeof document === "undefined") return "dark";
	const stored = readStoredMode();
	if (stored === "system") return "system";
	for (const modeClass of MODE_CLASSES) {
		if (document.documentElement.classList.contains(modeClass)) {
			return modeClass;
		}
	}
	return stored;
};

export const readStoredMode = (): ColorMode => {
	if (typeof window === "undefined") return "dark";
	const stored = window.localStorage.getItem(STORAGE_KEY_MODE);
	if (isColorMode(stored)) return stored;
	return "dark";
};
