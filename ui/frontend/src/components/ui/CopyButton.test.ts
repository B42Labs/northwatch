import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { flushSync, tick } from 'svelte';
import CopyButton from './CopyButton.svelte';
import { copyText } from '../../lib/copyView';

// copyText is exercised directly in lib/copyView.test.ts; here it is a spy so
// the button's own states are what the assertions see.
vi.mock('../../lib/copyView', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/copyView')>();
  return { ...actual, copyText: vi.fn(async () => {}) };
});

const copyTextMock = vi.mocked(copyText);

function button(): HTMLButtonElement {
  return screen.getByRole('button') as HTMLButtonElement;
}

/** Lets the click handler's awaited copyText settle before assertions. */
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

describe('CopyButton', () => {
  it('renders its label and accessible name', () => {
    render(CopyButton, {
      text: () => 'x',
      label: 'Copy MD',
      ariaLabel: 'Copy view as Markdown',
    });

    expect(button().textContent?.trim()).toBe('Copy MD');
    expect(button().getAttribute('aria-label')).toBe('Copy view as Markdown');
  });

  it('builds the text at click time, not on render', async () => {
    const text = vi.fn(() => 'payload');
    render(CopyButton, { text, label: 'Copy', ariaLabel: 'Copy' });

    expect(text).not.toHaveBeenCalled();

    await fireEvent.click(button());
    await settled();

    expect(text).toHaveBeenCalledTimes(1);
    expect(copyTextMock).toHaveBeenCalledWith('payload');
  });

  it('confirms with Copied and reverts after the revert window', async () => {
    render(CopyButton, { text: () => 'x', label: 'Copy', ariaLabel: 'Copy' });

    await fireEvent.click(button());
    await settled();
    expect(button().textContent?.trim()).toBe('Copied');

    vi.advanceTimersByTime(1500);
    flushSync();
    expect(button().textContent?.trim()).toBe('Copy');
  });

  it('reports Failed and logs when the clipboard rejects', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    copyTextMock.mockRejectedValue(new Error('clipboard unavailable'));
    render(CopyButton, { text: () => 'x', label: 'Copy', ariaLabel: 'Copy' });

    await fireEvent.click(button());
    await settled();

    expect(button().textContent?.trim()).toBe('Failed');
    expect(logged).toHaveBeenCalled();

    vi.advanceTimersByTime(1500);
    flushSync();
    expect(button().textContent?.trim()).toBe('Copy');
  });

  it('reports Failed when building the text throws', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    render(CopyButton, {
      text: () => {
        throw new TypeError('Converting circular structure to JSON');
      },
      label: 'Copy',
      ariaLabel: 'Copy',
    });

    await fireEvent.click(button());
    await settled();

    expect(button().textContent?.trim()).toBe('Failed');
    expect(copyTextMock).not.toHaveBeenCalled();
  });

  it('does not copy while disabled', async () => {
    render(CopyButton, {
      text: () => 'x',
      label: 'Copy',
      ariaLabel: 'Copy',
      disabled: true,
    });

    expect(button().disabled).toBe(true);

    await fireEvent.click(button());
    await settled();

    expect(copyTextMock).not.toHaveBeenCalled();
  });

  it('ignores a second click while the first copy is in flight', async () => {
    let release: () => void = () => {};
    copyTextMock.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    render(CopyButton, { text: () => 'x', label: 'Copy', ariaLabel: 'Copy' });

    await fireEvent.click(button());
    await tick();
    await fireEvent.click(button());
    await tick();

    expect(copyTextMock).toHaveBeenCalledTimes(1);

    release();
    await settled();
    expect(button().textContent?.trim()).toBe('Copied');
  });

  it('clears the pending revert when it unmounts', async () => {
    const { unmount } = render(CopyButton, {
      text: () => 'x',
      label: 'Copy',
      ariaLabel: 'Copy',
    });

    await fireEvent.click(button());
    await settled();
    expect(button().textContent?.trim()).toBe('Copied');

    unmount();
    // A timer left running would fire into a destroyed component here.
    expect(() => {
      vi.advanceTimersByTime(1500);
      flushSync();
    }).not.toThrow();
    expect(vi.getTimerCount()).toBe(0);
  });
});
