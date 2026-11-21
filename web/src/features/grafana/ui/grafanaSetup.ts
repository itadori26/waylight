import { t } from '../../../shared/i18n';
import type { FoundService, SetupDescriptor } from '../../../shared/setup/setup';
import type { GrafanaUseCases } from '../usecase/grafanaUseCases';

/** The setup assistant of the Grafana widgets (#618), as data. */
export function grafanaSetup(
  grafana: GrafanaUseCases,
  discover?: () => Promise<FoundService[]>,
): SetupDescriptor {
  return {
    discover,
    service: 'Grafana',
    fields: [
      {
        key: 'url',
        label: 'setup.address',
        stage: 'address',
        type: 'url',
        placeholder: 'http://192.168.178.20:3000',
      },
      { key: 'token', label: 'grafana.setup_token', stage: 'credential', type: 'password' },
    ],
    steps: [
      'grafana.setup_step_open',
      'grafana.setup_step_add',
      'grafana.setup_step_token',
      'grafana.setup_step_copy',
    ],
    credentialPage: (address) => (address ? `${address}/org/serviceaccounts/create` : null),
    async check(values) {
      await grafana.saveSettings(values['url'] ?? '', values['token'] ?? '');
      const a = await grafana.alerts();
      return { lines: [t('grafana.setup_alerts', { n: a.alerts.length })] };
    },
  };
}
