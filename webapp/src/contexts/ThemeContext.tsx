import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from 'react';

type ThemeMode = 'light' | 'dark' | 'system';

type Palette = 'taronja' | 'blue' | 'violet' | 'emerald';

type ThemeState = {
    mode: ThemeMode;
    palette: Palette;
    setMode: (mode: ThemeMode) => void;
    setPalette: (palette: Palette) => void;
};

const ThemeContext = createContext<ThemeState | undefined>(undefined);

const STORAGE_KEYS = {
    mode: 'taronja.admin.theme.mode',
    palette: 'taronja.admin.theme.palette',
} as const;

function getSystemTheme(): Exclude<ThemeMode, 'system'> {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function applyThemeAttributes(mode: ThemeMode, palette: Palette): void {
    const resolvedMode = mode === 'system' ? getSystemTheme() : mode;
    document.documentElement.setAttribute('data-theme', resolvedMode);
    document.documentElement.setAttribute('data-palette', palette);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
    const [mode, setMode] = useState<ThemeMode>(() => {
        const stored = localStorage.getItem(STORAGE_KEYS.mode);
        if (stored === 'light' || stored === 'dark' || stored === 'system') {
            return stored;
        }
        return 'system';
    });

    const [palette, setPalette] = useState<Palette>(() => {
        const stored = localStorage.getItem(STORAGE_KEYS.palette);
        if (stored === 'taronja' || stored === 'blue' || stored === 'violet' || stored === 'emerald') {
            return stored;
        }
        return 'taronja';
    });

    useEffect(() => {
        localStorage.setItem(STORAGE_KEYS.mode, mode);
        localStorage.setItem(STORAGE_KEYS.palette, palette);
        applyThemeAttributes(mode, palette);
    }, [mode, palette]);

    useEffect(() => {
        if (mode !== 'system') {
            return;
        }

        const media = window.matchMedia('(prefers-color-scheme: dark)');
        const handler = () => applyThemeAttributes(mode, palette);

        media.addEventListener('change', handler);
        return () => media.removeEventListener('change', handler);
    }, [mode, palette]);

    const value = useMemo<ThemeState>(() => ({
        mode,
        palette,
        setMode,
        setPalette,
    }), [mode, palette]);

    return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeState {
    const ctx = useContext(ThemeContext);
    if (!ctx) {
        throw new Error('useTheme must be used within ThemeProvider');
    }
    return ctx;
}
