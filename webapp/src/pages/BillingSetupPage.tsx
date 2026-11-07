import { Link } from 'react-router-dom';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { PageHeader } from '../components/ui/PageHeader';
import {
    useBillingSetup,
    useRunBillingSetupAction,
} from '../services/billing';

const isStripeLink = (url: string) => url.includes('stripe.com');

const paymentMethodNames: Record<string, string> = {
    acss_debit: 'Pre-authorized debit (ACSS)',
    alipay: 'Alipay',
    apple_pay: 'Apple Pay',
    bancontact: 'Bancontact',
    blik: 'BLIK',
    boleto: 'Boleto',
    card: 'Card',
    customer_balance: 'Bank transfer',
    eps: 'EPS',
    giropay: 'Giropay',
    google_pay: 'Google Pay',
    ideal: 'iDEAL',
    konbini: 'Konbini',
    link: 'Link',
    oxxo: 'OXXO',
    p24: 'Przelewy24',
    paypal: 'PayPal',
    sepa_debit: 'SEPA Direct Debit',
    sofort: 'Sofort',
    us_bank_account: 'US bank account',
    wechat_pay: 'WeChat Pay',
};

function PaymentMethodGroup({ title, types, tone }: { title: string; types: string[]; tone: 'success' | 'warning' }) {
    const color = tone === 'success' ? 'text-success' : 'text-warning';
    const bg = tone === 'success' ? 'bg-success/10' : 'bg-warning/10';
    return (
        <div className={`rounded-lg p-3 ${bg}`}>
            <p className={`text-xs font-semibold uppercase tracking-wide ${color}`}>{title}</p>
            <ul className="mt-2 flex flex-wrap gap-2">
                {types.length === 0 && <li className="text-sm text-muted-fg">None</li>}
                {types.map(type => (
                    <li key={type} className="text-sm rounded px-2 py-1 bg-surface">
                        {paymentMethodNames[type] ?? type}
                    </li>
                ))}
            </ul>
        </div>
    );
}

function PaymentMethodGroups({ immediate, deferred }: { immediate: string[]; deferred: string[] }) {
    return (
        <div className="space-y-3">
            <PaymentMethodGroup title="Immediate" types={immediate} tone="success" />
            <div className="space-y-2">
                <PaymentMethodGroup title="Deferred" types={deferred} tone="warning" />
                <p className="text-xs text-muted-fg">
                    Disable these in the Stripe dashboard (Settings → Payment methods) if you don't want them at checkout.
                </p>
            </div>
        </div>
    );
}

function StatusBadge({ status }: { status: string }) {
    const badges: Record<string, { bg: string; color: string; label: string }> = {
        ok: { bg: 'bg-success/10', color: 'text-success', label: 'OK' },
        check: { bg: 'bg-warning/10', color: 'text-warning', label: 'Atención' },
        missing: { bg: 'bg-danger/10', color: 'text-danger', label: 'Missing' },
        manual: { bg: 'bg-muted/10', color: 'text-muted-fg', label: 'By hand' },
    };

    const badge = badges[status] || badges.check;

    return (
        <span className={`px-3 py-1 rounded text-xs font-medium whitespace-nowrap shrink-0 ${badge.bg} ${badge.color}`}>
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
                                                title={isStripeLink(check.link) ? 'Opens the Stripe dashboard in a new tab' : undefined}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="text-sm text-primary hover:underline flex items-center gap-1"
                                            >
                                                {isStripeLink(check.link) ? 'Open in Stripe ↗' : 'More info ↗'}
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
                                        {check.id === 'stripe-payment-methods' && setup.data.paymentMethods && (
                                            <PaymentMethodGroups
                                                immediate={setup.data.paymentMethods.immediate}
                                                deferred={setup.data.paymentMethods.deferred}
                                            />
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
