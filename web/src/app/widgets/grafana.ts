import { t } from '../../shared/i18n';
import { lazy, type WidgetDefinition } from '../widgetKit';

/** The Grafana widgets (#574): one panel as an image or its values (#579), and the alerts. */
export const widgets: WidgetDefinition[] = [
  {
    type: {
      kind: 'grafana-panel',
      category: 'network',
      keywords: ['grafana', 'graph', 'chart', 'diagramm', 'monitoring', 'prometheus', 'metrics'],
      get title() {
        return t('widget.grafana-panel.title');
      },
      get description() {
        return t('widget.grafana-panel.description');
      },
      defaultSize: { w: 6, h: 4 },
    },
    icon: 'activity',
    view: lazy(
      () => import('../../features/grafana/ui/GrafanaPanelWidget.svelte'),
      (config, c) => ({ config, grafana: c.grafana }),
    ),
    settings: lazy(
      () => import('../../features/grafana/ui/GrafanaPanelSettings.svelte'),
      (config, c) => ({ config, grafana: c.grafana }),
    ),
  },
  {
    type: {
      kind: 'grafana-alerts',
      category: 'network',
      keywords: ['grafana', 'alerts', 'alarme', 'alerting', 'monitoring'],
      get title() {
        return t('widget.grafana-alerts.title');
      },
      get description() {
        return t('widget.grafana-alerts.description');
      },
      defaultSize: { w: 4, h: 4 },
    },
    icon: 'gauge',
    view: lazy(
      () => import('../../features/grafana/ui/GrafanaAlertsWidget.svelte'),
      (_config, c) => ({ grafana: c.grafana, discover: () => c.discovery.find('grafana') }),
    ),
  },
];
