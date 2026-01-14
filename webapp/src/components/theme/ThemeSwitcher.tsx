import { useTheme } from '../../contexts/ThemeContext';

const palettes = [
    { id: 'taronja', label: 'Taronja' },
    { id: 'blue', label: 'Blue' },
    { id: 'violet', label: 'Violet' },
    { id: 'emerald', label: 'Emerald' },
] as const;

export function ThemeSwitcher() {
    const { mode, palette, setMode, setPalette } = useTheme();

    return (
        <div className="flex items-center gap-2">
            <select
                aria-label="Theme"
                className="tg-input max-w-32 py-1.5"
                value={mode}
                onChange={(e) => setMode(e.target.value as 'light' | 'dark' | 'system')}
            >
                <option value="system">System</option>
                <option value="light">Light</option>
                <option value="dark">Dark</option>
            </select>

            <select
                aria-label="Palette"
                className="tg-input max-w-36 py-1.5"
                value={palette}
                onChange={(e) => setPalette(e.target.value as typeof palette)}
            >
                {palettes.map((p) => (
                    <option key={p.id} value={p.id}>
                        {p.label}
                    </option>
                ))}
            </select>
        </div>
    );
}
