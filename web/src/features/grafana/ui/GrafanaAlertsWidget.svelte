<script lang="ts">
  import SetupInPlace from '../../../shared/setup/SetupInPlace.svelte';
  import { grafanaSetup } from './grafanaSetup';
  import type { FoundService } from '../../../shared/setup/setup';
  import { usePolling } from '../../../shared/refresh';
  import { intlLocale, t } from '../../../shared/i18n';
  import { ApiError, errorMessage } from '../../../shared/api-client/http';
  import type { GrafanaAlerts } from '../domain/grafana';
  import type { GrafanaUseCases } from '../usecase/grafanaUseCases';

  /** Props of the Grafana alerts widget (#574). */
  interface Props {
    /** Grafana found in the home network for the setup (#627). */
    discover?: () => Promise<FoundService[]>;
    grafana: GrafanaUseCases;
  }
  let { grafana, discover }: Props = $props();
  const poll = usePolling();

  const POLL_MS = 60_000;
  let data = $state<GrafanaAlerts | null>(null);
  let error = $state('');
  let unconfigured = $state(false);

  const load = () =>
    grafana
      .alerts()
      .then((a) => ((data = a), (error = ''), (unconfigured = false)))
      .catch((err) => {
        unconfigured = err instanceof ApiError && err.status === 404;
        error = errorMessage(err);
        throw err;
      });
  $effect(() => poll(load, POLL_MS));
  // svelte-ignore state_referenced_locally
  const setup = grafanaSetup(grafana, discover);

  const since = (d: Date) =>
    d.toLocaleString(intlLocale(), { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
</script>

{#if unconfigured}
  <SetupInPlace
    text={t('grafana.connect_under_settings')}
    descriptor={setup}
    instance
    ondone={() => void load().catch(() => undefined)}
  />
{:else if error && !data}
  <p class="error">{error}</p>
{:else if data}
  {#if data.alerts.length === 0}
    <p class="muted">{t('grafana.all_quiet')}</p>
  {:else}
    <ul>
      {#each data.alerts as a, i (a.name + i)}
        <li class:pending={a.state === 'pending'}>
          <span class="dot" aria-hidden="true"></span>
          <div>
            <strong>{a.name}</strong>
            {#if a.state === 'pending'}<span class="muted"> · {t('grafana.pending')}</span>{/if}
            {#if a.summary}<div class="muted">{a.summary}</div>{/if}
            {#if a.since}<div class="muted small">{since(a.since)}</div>{/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
  {#if data.url}<a class="small" href={data.url} target="_blank" rel="noopener noreferrer"
      >{t('grafana.alert_list')}</a
    >{/if}
{:else}
  <p class="muted">{t('common.loading')}</p>
{/if}

<style>
  ul {
    list-style: none;
    margin: 0 0 var(--space-2);
    padding: 0;
    display: grid;
    gap: var(--space-2);
  }
  li {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
  }
  .dot {
    flex: none;
    width: 0.6rem;
    height: 0.6rem;
    border-radius: 50%;
    background: var(--color-danger);
  }
  .pending .dot {
    background: var(--color-warning, var(--color-accent));
  }
  .small {
    font-size: max(0.8rem, 0.75em); /* at least 12 px (#317) */
  }
</style>
