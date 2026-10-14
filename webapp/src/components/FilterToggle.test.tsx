import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { FilterToggle } from './FilterToggle';
import { toggleValue } from '../lib/toggleValue';

describe('FilterToggle', () => {
    it('describes what a click does in both states', () => {
        const off = renderToStaticMarkup(<FilterToggle label="Spain" active={false} onToggle={() => {}}>Spain</FilterToggle>);
        expect(off).toContain('aria-pressed="false"');
        expect(off).toContain('title="Filter by Spain"');
        const on = renderToStaticMarkup(<FilterToggle label="Spain" active onToggle={() => {}}>Spain</FilterToggle>);
        expect(on).toContain('aria-pressed="true"');
        expect(on).toContain('title="Clear filter: Spain"');
    });
});

describe('toggleValue', () => {
    it('selects a new value and clears the selected one', () => {
        expect(toggleValue('', 'Spain')).toBe('Spain');
        expect(toggleValue('France', 'Spain')).toBe('Spain');
        expect(toggleValue('Spain', 'Spain')).toBe('');
    });
});
