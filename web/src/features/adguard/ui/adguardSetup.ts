import { t } from '../../../shared/i18n';
import type { FoundService, SetupDescriptor } from '../../../shared/setup/setup';
import { type DnsProvider, providerName } from '../domain/adguard';
import type { AdGuardUseCases } from '../usecase/adguardUseCases';

/** The setup assistant of the DNS filter widget (#618), as data per provider. */
export function adguardSetup(
  adguard: AdGuardUseCases,
  provider: DnsProvider,
  discover?: () => Promise<FoundService[]>,
): SetupDescriptor {
  const pihole = provider === 'pihole';
  return {
    // Only AdGuard Home is found in the home network (#627).
    discover: pihole ? undefined : discover,
    service: providerName(provider),
    fields: [
      {
        key: 'url',
        label: 'setup.address',
        stage: 'address',
        type: 'url',
        placeholder: pihole ? 'http://192.168.178.53' : 'http://192.168.178.53:3000',
      },
      ...(pihole
        ? []
        : [{ key: 'username', label: 'adguard.setup_username', stage: 'credential', type: 'text' } as const]),
      {
        key: 'password',
        label: pihole ? 'adguard.setup_app_password' : 'adguard.setup_password',
        stage: 'credential',
        type: 'password',
        // A Pi-hole without a web password needs none.
        optional: pihole,
      },
    ],
    steps: pihole
      ? [
          'adguard.setup_step_pihole_open',
          'adguard.setup_step_pihole_create',
          'adguard.setup_step_pihole_copy',
        ]
      : ['adguard.setup_step_adguard_login'],
    credentialPage: (address) => (address && pihole ? `${address}/admin/settings/api` : null),
    async check(values) {
      await adguard.saveSettings(
        provider,
        values['url'] ?? '',
        values['username'] ?? '',
        values['password'] ?? '',
      );
      const s = await adguard.stats(provider);
      return {
        lines: [t('adguard.setup_found', { queries: s.queries, percent: Math.round(s.blockedPercent) })],
      };
    },
  };
}
