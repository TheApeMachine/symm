import {
	createContext,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useState,
} from "react";
import {
	applyAppearance,
	applyColorMode,
	applyContrast,
	applyVisualTheme,
	type ColorMode,
	isColorMode,
	isVisualTheme,
	type ResolvedColorMode,
	readBootstrappedContrast,
	readBootstrappedMode,
	readBootstrappedVisualTheme,
	readStoredContrast,
	readStoredMode,
	readStoredVisualTheme,
	resolveColorMode,
	resolveSystemColorMode,
	STORAGE_KEY_CONTRAST,
	STORAGE_KEY_MODE,
	STORAGE_KEY_VISUAL_THEME,
	type VisualTheme,
} from "#/lib/appearance";

type AppearanceContextValue = {
	mode: ColorMode;
	resolvedMode: ResolvedColorMode;
	setMode: (mode: ColorMode) => void;
	contrast: boolean;
	setContrast: (contrast: boolean) => void;
	visualTheme: VisualTheme;
	setVisualTheme: (visualTheme: VisualTheme) => void;
};

const AppearanceContext = createContext<AppearanceContextValue | null>(null);

export const ThemeProvider = ({ children }: { children: React.ReactNode }) => {
	const [mode, setModeState] = useState<ColorMode>(() =>
		readBootstrappedMode(),
	);
	const [contrast, setContrastState] = useState<boolean>(() =>
		readBootstrappedContrast(),
	);
	const [visualTheme, setVisualThemeState] = useState<VisualTheme>(() =>
		readBootstrappedVisualTheme(),
	);
	const [systemMode, setSystemMode] = useState<ResolvedColorMode>(() =>
		resolveSystemColorMode(),
	);

	useEffect(() => {
		const media = window.matchMedia("(prefers-color-scheme: dark)");
		const update = () => setSystemMode(media.matches ? "dark" : "light");
		media.addEventListener("change", update);
		return () => media.removeEventListener("change", update);
	}, []);

	const resolvedMode = mode === "system" ? systemMode : mode;

	useEffect(() => {
		applyAppearance({ mode, contrast, visualTheme });
	}, [mode, contrast, visualTheme]);

	const setMode = useCallback((next: ColorMode) => {
		window.localStorage.setItem(STORAGE_KEY_MODE, next);
		setModeState(next);
		applyColorMode(resolveColorMode(next));
	}, []);

	const setContrast = useCallback((next: boolean) => {
		window.localStorage.setItem(STORAGE_KEY_CONTRAST, next ? "1" : "0");
		setContrastState(next);
		applyContrast(next);
	}, []);

	const setVisualTheme = useCallback((next: VisualTheme) => {
		window.localStorage.setItem(STORAGE_KEY_VISUAL_THEME, next);
		setVisualThemeState(next);
		applyVisualTheme(next);
	}, []);

	const value = useMemo(
		() => ({
			mode,
			resolvedMode,
			setMode,
			contrast,
			setContrast,
			visualTheme,
			setVisualTheme,
		}),
		[
			mode,
			resolvedMode,
			setMode,
			contrast,
			setContrast,
			visualTheme,
			setVisualTheme,
		],
	);

	return (
		<AppearanceContext.Provider value={value}>
			{children}
		</AppearanceContext.Provider>
	);
};

export const useTheme = (): AppearanceContextValue => {
	const context = useContext(AppearanceContext);
	if (context === null) {
		throw new Error("useTheme must be used inside a ThemeProvider");
	}
	return context;
};

export {
	isColorMode,
	isVisualTheme,
	readStoredContrast,
	readStoredMode,
	readStoredVisualTheme,
};
