// toggleValue returns '' when value is already selected, so a second click clears it.
export function toggleValue(current: string, value: string): string {
    return current === value ? '' : value;
}
