import { Button } from './Button';
import { Card, CardContent, CardHeader } from './Card';

interface ConfirmDialogProps {
    open: boolean;
    title: string;
    description: string;
    confirmLabel?: string;
    cancelLabel?: string;
    variant?: 'primary' | 'danger';
    isLoading?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
}

// Shared in-app confirmation modal for destructive/impactful actions (token
// revocation, counter adjustments, etc.), so these no longer rely on the
// browser's native confirm() — which looks and behaves inconsistently with
// the rest of the admin UI and can't show a loading state.
export function ConfirmDialog({
    open,
    title,
    description,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    variant = 'primary',
    isLoading = false,
    onConfirm,
    onCancel,
}: ConfirmDialogProps) {
    if (!open) return null;

    return (
        <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="confirm-dialog-title"
            aria-describedby="confirm-dialog-description"
        >
            <Card className="w-full max-w-md">
                <CardHeader>
                    <h3 id="confirm-dialog-title" className="text-lg font-semibold">
                        {title}
                    </h3>
                </CardHeader>
                <CardContent>
                    <p id="confirm-dialog-description" className="text-sm text-muted-fg">
                        {description}
                    </p>
                    <div className="mt-6 flex justify-end gap-3">
                        <Button type="button" variant="secondary" onClick={onCancel} disabled={isLoading}>
                            {cancelLabel}
                        </Button>
                        <Button type="button" variant={variant} onClick={onConfirm} disabled={isLoading}>
                            {isLoading ? 'Working…' : confirmLabel}
                        </Button>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
