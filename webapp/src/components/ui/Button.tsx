import { ButtonHTMLAttributes } from 'react';
import { cn } from '../../lib/cn';

type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';

type ButtonSize = 'sm' | 'md';

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: ButtonVariant;
    size?: ButtonSize;
};

const base = 'inline-flex items-center justify-center gap-2 rounded-lg text-sm font-medium transition-colors outline-none focus:ring-2 focus:ring-ring/40 disabled:opacity-50 disabled:pointer-events-none';

const variants: Record<ButtonVariant, string> = {
    primary: 'bg-primary text-primary-fg hover:bg-primary/90',
    secondary: 'bg-muted text-fg hover:bg-muted/80',
    outline: 'border border-border bg-transparent text-fg hover:bg-muted/60',
    ghost: 'bg-transparent text-fg hover:bg-muted/60',
    danger: 'bg-danger text-danger-fg hover:bg-danger/90',
};

const sizes: Record<ButtonSize, string> = {
    sm: 'h-9 px-3',
    md: 'h-10 px-4',
};

export function Button({ className, variant = 'primary', size = 'md', ...props }: Props) {
    return (
        <button
            className={cn(base, variants[variant], sizes[size], className)}
            {...props}
        />
    );
}
