import { ReactNode } from 'react';
import { cn } from '../../lib/cn';

type Props = {
    title: string;
    description?: string;
    actions?: ReactNode;
    className?: string;
};

export function PageHeader({ title, description, actions, className }: Props) {
    return (
        <div
            className={cn(
                'flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between',
                className
            )}
        >
            <div>
                <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
                {description && <p className="mt-1 text-sm text-muted-fg">{description}</p>}
            </div>
            {actions && <div className="flex items-center gap-3">{actions}</div>}
        </div>
    );
}
