import { ReactNode } from 'react';
import { cn } from '../../lib/cn';

type Props = {
    label: string;
    htmlFor?: string;
    description?: string;
    error?: string;
    required?: boolean;
    className?: string;
    children: ReactNode;
};

export function FormField({
    label,
    htmlFor,
    description,
    error,
    required = false,
    className,
    children,
}: Props) {
    return (
        <div className={cn('space-y-2', className)}>
            <label htmlFor={htmlFor} className="block text-sm font-medium text-muted-fg">
                {label}
                {required && <span className="text-danger"> *</span>}
            </label>
            {children}
            {description && !error && <p className="text-xs text-muted-fg">{description}</p>}
            {error && <p className="text-xs text-danger">{error}</p>}
        </div>
    );
}
