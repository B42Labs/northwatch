import { describe, it, expect } from 'vitest';
import { correlationStatus, upVariant } from './ovsCorrelate';
import type { OvsInterfaceCorrelation } from './api';

describe('correlationStatus', () => {
  it('flags an interface with no iface-id as not OVN-managed', () => {
    expect(correlationStatus({ bound: false })).toEqual({
      label: 'not OVN-managed',
      variant: 'neutral',
    });
  });

  it('warns when an iface-id has no Southbound Port_Binding', () => {
    expect(correlationStatus({ iface_id: 'lsp-a', bound: false })).toEqual({
      label: 'no SB Port_Binding',
      variant: 'warning',
    });
  });

  it('flags drift as an error even when a binding is present', () => {
    const c: OvsInterfaceCorrelation = {
      iface_id: 'lsp-a',
      bound: true,
      binding: { uuid: 'u', logical_port: 'lsp-a', bound_here: true },
      drift: [
        'SB reports the port up but the OVS interface link_state is down',
      ],
    };
    expect(correlationStatus(c)).toEqual({ label: 'drift', variant: 'error' });
  });

  it('warns when the port is bound on a different chassis', () => {
    const c: OvsInterfaceCorrelation = {
      iface_id: 'lsp-a',
      bound: true,
      binding: {
        uuid: 'u',
        logical_port: 'lsp-a',
        bound_here: false,
        chassis: 'other',
      },
    };
    expect(correlationStatus(c)).toEqual({
      label: 'bound on other',
      variant: 'warning',
    });
  });

  it('reports a healthy local binding as bound/success', () => {
    const c: OvsInterfaceCorrelation = {
      iface_id: 'lsp-a',
      bound: true,
      binding: {
        uuid: 'u',
        logical_port: 'lsp-a',
        bound_here: true,
        chassis: 'chassis-1',
      },
    };
    expect(correlationStatus(c)).toEqual({
      label: 'bound',
      variant: 'success',
    });
  });
});

describe('upVariant', () => {
  it('maps true to success and false/undefined to neutral', () => {
    expect(upVariant(true)).toBe('success');
    expect(upVariant(false)).toBe('neutral');
    expect(upVariant(undefined)).toBe('neutral');
  });
});
