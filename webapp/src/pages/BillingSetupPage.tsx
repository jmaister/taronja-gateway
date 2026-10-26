import { Link } from 'react-router-dom';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { PageHeader } from '../components/ui/PageHeader';
import {
    useBillingSetup,
    useRunBillingSetupAction,
} from '../services/billing';

function StatusBadge({ status }: { status: string }) {
    const badges: Record<string, { bg: string; color: string; label: string }> = {
        ok: { bg: 'bg-success/10', color: 'text-success', label: 'OK' },
        check: { bg: 'bg-warning/10', color: 'text-warning', label: 'Check' },
        missing: { bg: 'bg-danger/10', color: 'text-danger', label: 'Missing' },
        manual: { bg: 'bg-muted/10', color: 'text-muted-fg', label: 'By hand' },
    };

    const badge = badges[status] || badges.check;

    return (
        <span className={`px-3 py-1 rounded text-xs font-medium ${badge.bg} ${badge.color}`}>
            {badge.label}
        </span>
    );
}

export function BillingSetupPage() {
    const setup = useBillingSetup();
    const runAction = useRunBillingSetupAction();

    const handleRunAction = (action: string) => {
        runAction.mutate({ action });
    };

    return (
        <div className="space-y-6">
            <PageHeader
                title="Billing Setup"
                description="Check and configure billing integration requirements"
            />

            <QueryState isLoading={setup.isLoading} error={setup.error} onRetry={() => setup.refetch()}>
                {setup.data && (
                    <div className="space-y-4">
                        {setup.data.checks.map(check => (
                            <Card key={check.id}>
                                <CardHeader>
                                    <div className="flex items-start justify-between gap-4">
                                        <div>
                                            <h3 className="font-semibold">{check.title}</h3>
                                            <p className="text-sm text-muted-fg mt-1">{check.detail}</p>
                                        </div>
                                        <StatusBadge status={check.status} />
                                    </div>
                                </CardHeader>
                                <CardContent>
                                    <div className="space-y-3">
                                        {check.link && check.link.startsWith('/') && (
                                            <Link to={check.link} className="text-sm text-primary hover:underline">
                                                Open the documents →
                                            </Link>
                                        )}
                                        {check.link && !check.link.startsWith('/') && (
                                            <a
                                                href={check.link}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="text-sm text-primary hover:underline flex items-center gap-1"
                                            >
                                                More info ↗
                                            </a>
                                        )}
                                        {check.action && (
                                            <Button
                                                size="sm"
                                                onClick={() => handleRunAction(check.action!)}
                                                disabled={runAction.isPending}
                                            >
                                                {runAction.isPending ? 'Running…' : check.action}
                                            </Button>
                                        )}
                                        {runAction.error && (
                                            <p className="text-sm text-danger">{runAction.error.message}</p>
                                        )}
                                    </div>
                                </CardContent>
                            </Card>
                        ))}
                    </div>
                )}
            </QueryState>
        </div>
    );
}
