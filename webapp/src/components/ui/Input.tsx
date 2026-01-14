import { InputHTMLAttributes } from 'react';
import { cn } from '../../lib/cn';

type Props = InputHTMLAttributes<HTMLInputElement>;

export function Input({ className, ...props }: Props) {
    return <input className={cn('tg-input', className)} {...props} />;
}
