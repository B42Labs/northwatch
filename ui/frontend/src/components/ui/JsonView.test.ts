import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { flushSync, tick } from 'svelte';
import JsonView from './JsonView.svelte';
import { copyText } from '../../lib/copyView';

vi.mock('../../lib/copyView', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/copyView')>();
  return { ...actual, copyText: vi.fn(async () => {}) };
});

const copyTextMock = vi.mocked(copyText);

function copyButton(): HTMLButtonElement {
  return screen.getByRole('button', { name: 'Copy JSON' }) as HTMLButtonElement;
}

function toggleButton(): HTMLButtonElement {
  return screen.getByRole('button', { expanded: false }) as HTMLButtonElement;
}

async function settled() {
  await tick();
  await Promise.resolve();
  flushSync();
}

beforeEach(() => {
  vi.useFakeTimers();
  copyTextMock.mockReset();
  copyTextMock.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('JsonView', () => {
  it('renders the toggle and the copy action side by side', () => {
    render(JsonView, { data: { a: 1 }, label: 'Raw JSON' });

    const toggle = toggleButton();
    expect(toggle.textContent).toContain('Raw JSON');
    expect(copyButton()).toBeTruthy();
    // A button inside a button would be invalid markup.
    expect(copyButton().closest('button')).toBe(copyButton());
    expect(toggle.querySelector('button')).toBeNull();
  });

  it('expands only on the toggle, not on copy', async () => {
    render(JsonView, { data: { a: 1 } });

    await fireEvent.click(copyButton());
    await settled();
    expect(screen.queryByRole('button', { expanded: true })).toBeNull();

    await fireEvent.click(toggleButton());
    flushSync();
    expect(screen.getByRole('button', { expanded: true })).toBeTruthy();
  });

  it('copies the record as indented JSON and confirms', async () => {
    const data = { name: 'sw0', ports: 2 };
    render(JsonView, { data });

    await fireEvent.click(copyButton());
    await settled();

    expect(copyTextMock).toHaveBeenCalledWith(JSON.stringify(data, null, 2));
    expect(copyButton().textContent?.trim()).toBe('Copied');

    vi.advanceTimersByTime(1500);
    flushSync();
    expect(copyButton().textContent?.trim()).toBe('Copy');
  });

  it('reports Failed when the clipboard rejects', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    copyTextMock.mockRejectedValue(new Error('clipboard unavailable'));
    render(JsonView, { data: { a: 1 } });

    await fireEvent.click(copyButton());
    await settled();

    expect(copyButton().textContent?.trim()).toBe('Failed');
  });

  it('copies null for an absent record', async () => {
    render(JsonView, { data: null });

    await fireEvent.click(copyButton());
    await settled();

    expect(copyTextMock).toHaveBeenCalledWith('null');
  });
});
