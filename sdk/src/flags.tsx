import { useEffect, useState } from 'react';
import { useTaronjaClient } from './auth';

export interface FeatureFlagsState {
    flags: Record<string, boolean>;
    isLoading: boolean;
    error: Error | null;
}

/**
 * Loads every feature flag value from the gateway once per mount. Must be
 * used inside TaronjaAuthProvider.
 */
export function useFeatureFlags(): FeatureFlagsState {
    const client = useTaronjaClient();
    const [state, setState] = useState<FeatureFlagsState>({ flags: {}, isLoading: true, error: null });

    useEffect(() => {
        const controller = new AbortController();
        client.getFeatureFlags({ signal: controller.signal }).then(
            flags => setState({ flags, isLoading: false, error: null }),
            (error: unknown) => {
                if (controller.signal.aborted) return;
                setState({ flags: {}, isLoading: false, error: error instanceof Error ? error : new Error(String(error)) });
            },
        );
        return () => controller.abort();
    }, [client]);

    return state;
}

/**
 * Whether one flag is on. Returns `defaultValue` while loading, on error, and
 * for a key the gateway does not know.
 */
export function useFeatureFlag(key: string, defaultValue = false): boolean {
    const { flags } = useFeatureFlags();
    return flags[key] ?? defaultValue;
}
