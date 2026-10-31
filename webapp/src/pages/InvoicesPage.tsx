import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import {
    useBillingDocuments,
    useRetryBillingDocument,
} from '../services/billing';

const formatter = new Intl.NumberFormat('es-ES', {
    style: 'currency',
    currency: 'EUR',
});

function formatPrice(cents: number) {
    return formatter.format(cents / 100);
}

export function InvoicesPage() {
    const [params] = useSearchParams();
    const [filters, setFilters] = useState({
        subject: params.get('subject') ?? '',
        state: params.get('state') ?? '',
        kind: params.get('kind') ?? '',
    });

    const documents = useBillingDocuments(
        Object.values(filters).some(v => v) ? { subject: filters.subject || undefined, state: filters.state || undefined, kind: filters.kind || undefined } : undefined,
    );

    const retry = useRetryBillingDocument();

    const handleOpenPdf = (id: string) => {
        const baseUrl = '/_/api/billing/documents';
        window.open(`${baseUrl}/${id}/pdf`, '_blank');
    };

    return (
        <div className="space-y-6">
            <PageHeader
                title="Invoices & Credit Notes"
                description="Manage billing documents"
            />

            <Card>
                <CardHeader>
                    <h3 className="font-semibold">Filters</h3>
                </CardHeader>
                <CardContent>
                    <div className="grid gap-4 grid-cols-1 md:grid-cols-3">
                        <FormField label="Subject" htmlFor="filter-subject">
                            <Input
                                id="filter-subject"
                                value={filters.subject}
                                onChange={e => setFilters({ ...filters, subject: e.target.value })}
                                placeholder="user:123 or org:456"
                            />
                        </FormField>
                        <FormField label="State" htmlFor="filter-state">
                            <select
                                id="filter-state"
                                value={filters.state}
                                onChange={e => setFilters({ ...filters, state: e.target.value })}
                                className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                            >
                                <option value="">All states</option>
                                <option value="pending">Pending</option>
                                <option value="done">Done</option>
                                <option value="failed">Failed</option>
                            </select>
                        </FormField>
                        <FormField label="Kind" htmlFor="filter-kind">
                            <select
                                id="filter-kind"
                                value={filters.kind}
                                onChange={e => setFilters({ ...filters, kind: e.target.value })}
                                className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                            >
                                <option value="">All kinds</option>
                                <option value="invoice">Invoice</option>
                                <option value="credit_note">Credit Note</option>
                            </select>
                        </FormField>
                    </div>
                </CardContent>
            </Card>

            <QueryState isLoading={documents.isLoading} error={documents.error} onRetry={() => documents.refetch()}>
                <Card>
                    {documents.data && documents.data.length === 0 ? (
                        <CardContent className="text-sm text-muted-fg">No documents found.</CardContent>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-border text-left text-muted-fg">
                                        <th className="px-4 py-3 font-medium">Kind</th>
                                        <th className="px-4 py-3 font-medium">Number</th>
                                        <th className="px-4 py-3 font-medium">Subject</th>
                                        <th className="px-4 py-3 font-medium">Amount</th>
                                        <th className="px-4 py-3 font-medium">Odoo State</th>
                                        <th className="px-4 py-3 font-medium">Paid At</th>
                                        <th className="px-4 py-3 font-medium">Error</th>
                                        <th className="px-4 py-3 font-medium">Attempts</th>
                                        <th className="px-4 py-3" />
                                    </tr>
                                </thead>
                                <tbody>
                                    {documents.data?.map(doc => (
                                        <tr key={doc.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                                            <td className="px-4 py-3">
                                                <span className={`px-2 py-1 rounded text-xs font-medium ${
                                                    doc.kind === 'invoice' ? 'bg-blue/10 text-blue' : 'bg-green/10 text-green'
                                                }`}>
                                                    {doc.kind}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 font-mono">{doc.number}</td>
                                            <td className="px-4 py-3 font-mono text-xs">{doc.subject}</td>
                                            <td className="px-4 py-3 font-mono">{formatPrice(doc.amountCents)}</td>
                                            <td className="px-4 py-3">
                                                <span className={`px-2 py-1 rounded text-xs font-medium ${
                                                    doc.odooState === 'done' ? 'bg-success/10 text-success' :
                                                    doc.odooState === 'failed' ? 'bg-danger/10 text-danger' :
                                                    'bg-warning/10 text-warning'
                                                }`}>
                                                    {doc.odooState}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {doc.paidAt ? new Date(doc.paidAt).toLocaleDateString() : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {doc.blockedReason && (
                                                    <div className="max-w-xs text-warning">{doc.blockedReason}</div>
                                                )}
                                                {doc.odooError ? (
                                                    <div className="max-w-xs truncate" title={doc.odooError}>
                                                        {doc.odooError}
                                                    </div>
                                                ) : !doc.blockedReason && '—'}
                                            </td>
                                            <td className="px-4 py-3 text-center">
                                                {doc.attempts}
                                            </td>
                                            <td className="px-4 py-3 text-right space-x-2">
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => handleOpenPdf(doc.id)}
                                                    title="Open PDF"
                                                >
                                                    📄
                                                </Button>
                                                {doc.invoiceDocumentId && doc.blockedReason && (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => retry.mutate(doc.invoiceDocumentId!)}
                                                        disabled={retry.isPending}
                                                    >
                                                        Retry its invoice
                                                    </Button>
                                                )}
                                                {(doc.odooState === 'failed' || doc.odooState === 'pending') && (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => retry.mutate(doc.id)}
                                                        disabled={retry.isPending}
                                                    >
                                                        Retry
                                                    </Button>
                                                )}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </Card>
            </QueryState>

            {retry.error && <Card className="bg-danger/10 border-danger/20"><CardContent className="text-sm text-danger">{retry.error.message}</CardContent></Card>}
        </div>
    );
}
