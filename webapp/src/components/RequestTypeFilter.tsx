// Filter control for RequestDetail.is_static (see management.excludeStaticAssets
// in the gateway config): whether a request was for a static asset
// (CSS/JS/images/fonts/...) or something dynamic (a page, an API call).
export type RequestTypeFilterValue = "all" | "static" | "dynamic";

const OPTIONS: { value: RequestTypeFilterValue; label: string }[] = [
    { value: "all", label: "All requests" },
    { value: "dynamic", label: "Non-static only" },
    { value: "static", label: "Static assets only" },
];

// Converts the UI's three-way choice to the is_static query param the API
// expects: undefined (both), true (static only), or false (non-static only).
export function requestTypeFilterToIsStatic(value: RequestTypeFilterValue): boolean | undefined {
    if (value === "static") return true;
    if (value === "dynamic") return false;
    return undefined;
}

export function RequestTypeFilter({
    value,
    onChange,
}: {
    value: RequestTypeFilterValue;
    onChange: (value: RequestTypeFilterValue) => void;
}) {
    return (
        <div>
            <label className="block text-sm font-medium text-muted-fg">Request type</label>
            <select
                className="tg-input py-1.5"
                value={value}
                onChange={(e) => onChange(e.target.value as RequestTypeFilterValue)}
            >
                {OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>
                        {o.label}
                    </option>
                ))}
            </select>
        </div>
    );
}
