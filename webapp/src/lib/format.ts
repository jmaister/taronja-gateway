import { getSafeLocale } from './intl';

export function formatNumber(value: number, options?: Intl.NumberFormatOptions): string {
    return new Intl.NumberFormat(getSafeLocale(), options).format(value);
}

export function formatDateTime(value: string | number | Date | null | undefined, fallback = 'Never'): string {
    if (value === null || value === undefined || value === '') return fallback;
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return fallback;
    return date.toLocaleString(getSafeLocale());
}

export function formatDate(value: string | number | Date | null | undefined, fallback = 'Never'): string {
    if (value === null || value === undefined || value === '') return fallback;
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return fallback;
    return date.toLocaleDateString(getSafeLocale());
}

export function formatBytes(bytes: number): string {
    if (bytes <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}
