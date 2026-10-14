import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { QueryState } from './QueryState';

describe('QueryState', () => {
    it('renders children when idle', () => {
        expect(renderToStaticMarkup(<QueryState isLoading={false} error={null}>ready</QueryState>)).toContain('ready');
    });

    it('renders the error instead of children', () => {
        const html = renderToStaticMarkup(
            <QueryState isLoading={false} error={new Error('boom')} onRetry={() => {}}>ready</QueryState>,
        );
        expect(html).toContain('boom');
        expect(html).toContain('Retry');
        expect(html).not.toContain('ready');
    });

    it('renders the loading text', () => {
        const html = renderToStaticMarkup(<QueryState isLoading error={null} loadingText="Wait">ready</QueryState>);
        expect(html).toContain('Wait');
        expect(html).not.toContain('ready');
    });
});
