import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { flushSync, tick } from 'svelte';
import CopyViewButtons from './CopyViewButtons.svelte';
import { copyText, maskEnabled, renderJSON } from '../../lib/copyView';
import { location as routeLocation } from '../../lib/router';
import { clusters, activeCluster } from '../../lib/clusterStore';

// Only the clipboard write is replaced; the renderers and the masking stay real
// so the copied text is the text an operator would paste.
vi.mock('../../lib/copyView', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/copyView')>();
  return { ...actual, copyText: vi.fn(async () => {}) };
});

const copyTextMock = vi.mocked(copyText);

function mdButton(): HTMLButtonElement {
  return screen.getByRole('button', {
    name: 'Copy view as Markdown',
  }) as HTMLButtonElement;
}

function jsonButton(): HTMLButtonElement {
  return screen.getByRole('button', {
    name: 'Copy view as JSON',
  }) as HTMLButtonElement;
}

function maskBox(): HTMLInputElement {
  return screen.getByRole('checkbox') as HTMLInputElement;
}

async function settled() {
  await tick();
  await Promise.resolve();
  flushSync();
}

const VIEW = { addresses: ['0a:00:00:00:00:01 10.0.0.5'], total: 1 };

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-09-22T08:30:00.000Z'));
  copyTextMock.mockReset();
  copyTextMock.mockResolvedValue(undefined);
  maskEnabled.set(false);
  clusters.set([{ name: 'prod', label: 'Production', ready: true }]);
  activeCluster.set('prod');
  routeLocation.set('/debug/port-diagnostics?severity=error');
});

afterEach(() => {
  vi.useRealTimers();
  maskEnabled.set(false);
});

describe('CopyViewButtons', () => {
  it('renders the mask toggle and both copy actions', () => {
    render(CopyViewButtons, { title: 'Port Diagnostics', view: () => VIEW });

    expect(maskBox()).toBeTruthy();
    expect(mdButton().textContent?.trim()).toBe('Copy MD');
    expect(jsonButton().textContent?.trim()).toBe('Copy JSON');
  });

  it('copies Markdown with the context header', async () => {
    render(CopyViewButtons, { title: 'Port Diagnostics', view: () => VIEW });

    await fireEvent.click(mdButton());
    await settled();

    const copied = copyTextMock.mock.calls[0][0];
    expect(copied).toContain('## Northwatch view: Port Diagnostics');
    expect(copied).toContain('- Cluster: prod (Production)');
    expect(copied).toContain('- Mode: live');
    expect(copied).toContain('- Route: /debug/port-diagnostics');
    expect(copied).toContain('- Query: severity=error');
    expect(copied).toContain('- Copied at: 2026-09-22T08:30:00.000Z');
    expect(copied).toContain('- Masked: no');
    expect(copied).toContain(renderJSON(VIEW));
  });

  it('copies raw JSON with no header', async () => {
    render(CopyViewButtons, { title: 'Port Diagnostics', view: () => VIEW });

    await fireEvent.click(jsonButton());
    await settled();

    expect(copyTextMock).toHaveBeenCalledWith(renderJSON(VIEW));
  });

  it('masks the payload and says so when the toggle is on', async () => {
    render(CopyViewButtons, { title: 'Port Diagnostics', view: () => VIEW });

    await fireEvent.click(maskBox());
    await fireEvent.click(mdButton());
    await settled();

    const copied = copyTextMock.mock.calls[0][0];
    expect(copied).toContain('- Masked: yes');
    expect(copied).toContain('mac-1 ip-1');
    expect(copied).not.toContain('10.0.0.5');
  });

  it('reads the view once per click, at click time', async () => {
    const view = vi.fn(() => VIEW);
    render(CopyViewButtons, { title: 'Port Diagnostics', view });

    expect(view).not.toHaveBeenCalled();

    await fireEvent.click(jsonButton());
    await settled();

    expect(view).toHaveBeenCalledTimes(1);
  });

  it('confirms on the clicked button only', async () => {
    render(CopyViewButtons, { title: 'Port Diagnostics', view: () => VIEW });

    await fireEvent.click(mdButton());
    await settled();

    expect(mdButton().textContent?.trim()).toBe('Copied');
    expect(jsonButton().textContent?.trim()).toBe('Copy JSON');

    vi.advanceTimersByTime(1500);
    flushSync();
    expect(mdButton().textContent?.trim()).toBe('Copy MD');
  });

  it('disables both actions while the view has no data', async () => {
    render(CopyViewButtons, {
      title: 'Port Diagnostics',
      view: () => null,
      disabled: true,
    });

    expect(mdButton().disabled).toBe(true);
    expect(jsonButton().disabled).toBe(true);

    await fireEvent.click(mdButton());
    await settled();

    expect(copyTextMock).not.toHaveBeenCalled();
  });
});
