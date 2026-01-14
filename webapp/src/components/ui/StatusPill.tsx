import { HTMLAttributes } from 'react';
import { cn } from '../../lib/cn';

type StatusVariant = 'default' | 'success' | 'warning' | 'danger' | 'info';

type Props = HTMLAttributes<HTMLSpanElement> & {
    variant?: StatusVariant;
};

const variants: Record<StatusVariant, string> = {
    default: 'bg-muted text-muted-fg',
    success: 'bg-success/15 text-success',
    warning: 'bg-warning/15 text-warning',
    danger: 'bg-danger/15 text-danger',
    info: 'bg-primary/15 text-primary',
};

export function StatusPill({ className, variant = 'default', ...props }: Props) {
    return (
        <span
            className={cn('inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold', variants[variant], className)}
            {...props}
        />
    );
}
