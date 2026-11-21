import { t } from '../../../shared/i18n';
import type { FoundService, SetupDescriptor } from '../../../shared/setup/setup';
import type { FritzBoxUseCases } from '../usecase/fritzboxUseCases';

/** The web interface of the box: same host as TR-064, default port. */
function webInterface(address: string): string | null {
  try {
    const u = new URL(address);
    return `${u.protocol}//${u.hostname}/`;
  } catch {
    return null;
  }
}

/**
 * The setup assistant of the FRITZ!Box widgets (#618), as data. The internet
 * widget works with UPnP alone, so its login is optional; the call list
 * needs a FRITZ!Box user.
 */
export function fritzboxSetup(
  fritzbox: FritzBoxUseCases,
  calls: boolean,
  discover?: () => Promise<FoundService[]>,
): SetupDescriptor {
  return {
    discover,
    service: 'FRITZ!Box',
    fields: [
      {
        key: 'url',
        label: 'setup.address',
        stage: 'address',
        type: 'url',
        placeholder: 'http://192.168.178.1:49000',
      },
      {
        key: 'username',
        label: 'fritzbox.setup_username',
        stage: 'credential',
        type: 'text',
        optional: !calls,
      },
      {
        key: 'password',
        label: 'fritzbox.setup_password',
        stage: 'credential',
        type: 'password',
        optional: !calls,
      },
    ],
    steps: calls
      ? ['fritzbox.setup_step_network', 'fritzbox.setup_step_user_calls', 'fritzbox.setup_step_enter']
      : ['fritzbox.setup_step_network', 'fritzbox.setup_step_user', 'fritzbox.setup_step_enter_optional'],
    credentialPage: (address) => (address ? webInterface(address) : null),
    async check(values) {
      await fritzbox.saveSettings(values['url'] ?? '', values['username'] ?? '', values['password'] ?? '');
      const s = await fritzbox.status();
      const lines = [
        s.model,
        s.wan.connected
          ? t('fritzbox.setup_online', { ip: s.wan.externalIp || '–' })
          : t('fritzbox.setup_offline'),
      ];
      if (calls) {
        const list = await fritzbox.calls({ filter: 'all', limit: 5, showNumbers: false });
        lines.push(t('fritzbox.setup_calls', { n: list.calls.length }));
      }
      return { lines: lines.filter((l) => l !== '') };
    },
  };
}
