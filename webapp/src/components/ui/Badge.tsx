import { HTMLAttributes } from 'react';
import { cn } from '../../lib/cn';

type BadgeVariant = 'default' | 'success' | 'warning' | 'danger';

type Props = HTMLAttributes<HTMLSpanElement> & {
    variant?: BadgeVariant;
};

const variants: Record<BadgeVariant, string> = {
    default: 'bg-muted text-fg',
    success: 'bg-success text-success-fg',
    warning: 'bg-warning text-warning-fg',
    danger: 'bg-danger text-danger-fg',
};

export function Badge({ className, variant = 'default', ...props }: Props) {
    return (
        <span
            className={cn('inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium', variants[variant], className)}
            {...props}
        />
    );
}
