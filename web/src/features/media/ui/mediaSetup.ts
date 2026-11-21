import { t } from '../../../shared/i18n';
import type { FoundService, SetupDescriptor } from '../../../shared/setup/setup';
import type { MediaProvider } from '../domain/media';
import type { MediaUseCases } from '../usecase/mediaUseCases';

/** The setup assistant of the media server widget (#618), as data. */
export function mediaSetup(media: MediaUseCases, discover?: () => Promise<FoundService[]>): SetupDescriptor {
  return {
    // Only Jellyfin is found; the provider choice stays on its first option.
    discover,
    service: 'Jellyfin / Plex',
    fields: [
      {
        key: 'provider',
        label: 'media.server',
        stage: 'address',
        type: 'choice',
        options: [
          { value: 'jellyfin', label: 'Jellyfin' },
          { value: 'plex', label: 'Plex' },
        ],
      },
      {
        key: 'url',
        label: 'setup.address',
        stage: 'address',
        type: 'url',
        placeholder: 'http://192.168.178.20:8096',
      },
      { key: 'token', label: 'media.setup_token', stage: 'credential', type: 'password' },
    ],
    steps: ['media.setup_step_jellyfin', 'media.setup_step_plex', 'media.setup_step_copy'],
    // Plex has no page that shows the token; its step explains where to find it.
    credentialPage: (address, values) =>
      address && values['provider'] !== 'plex' ? `${address}/web/#/dashboard/keys` : null,
    async check(values) {
      await media.saveSettings(
        (values['provider'] ?? 'jellyfin') as MediaProvider,
        values['url'] ?? '',
        values['token'] ?? '',
      );
      const o = await media.overview();
      return {
        lines: [
          o.serverName,
          t('media.setup_found', { recent: o.recent.length, playing: o.sessions.length }),
        ].filter((l) => l !== ''),
      };
    },
  };
}
