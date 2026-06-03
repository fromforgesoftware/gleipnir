import { Cable } from '@lucide/vue';
import type { ForgeConsolePlugin } from '@fromforgesoftware/forge-console-plugin';
import {
	ResourceListView,
	ResourceCreateForm,
} from '@fromforgesoftware/forge-console-plugin/ui';

// PluginContext is what the forge-console-plugin loader passes to a remote
// module's default-export factory. apiBase is resolved at RUNTIME from the
// backend /apps descriptor (descriptor.apiBase), not at build time — this
// module.js is built once, before any deployment knows its gateway base.
export interface PluginContext {
	apiBase: string;
}

// gleipnirPlugin builds the ForgeConsolePlugin for a given apiBase: the
// read-only provider/connector catalog, the owner's authorized connections
// (with lifecycle status), and a form to authorize a new connection. Credential
// intake is API/CLI-only (it carries secrets), so there is no secret-bearing
// form here. In the forge host this used to call apiBaseFor('gleipnir') at
// construction; in the remote module the apiBase is injected by the loader via
// the factory below.
export function gleipnirPlugin(apiBase: string): ForgeConsolePlugin {
	return {
		serviceId: 'gleipnir',
		type: 'app',
		title: 'Gleipnir',
		basePath: '/gleipnir',
		apiBase,
		icon: Cable,
		order: 4,
		pages: [
			{
				path: 'connections',
				name: 'Connections',
				component: ResourceListView,
				props: {
					apiBase,
					type: 'connections',
					title: 'Connections',
					columns: ['owner', 'connector', 'status', 'expiresAt'],
				},
			},
			{
				path: 'connections/new',
				name: 'New connection',
				component: ResourceCreateForm,
				props: {
					apiBase,
					type: 'connections',
					title: 'Authorize a connection',
					fields: [
						{ name: 'owner', label: 'Owner', required: true },
						{
							name: 'connector',
							label: 'Connector',
							type: 'select',
							options: [
								{ value: 'alpaca', label: 'Alpaca' },
								{ value: 'binance', label: 'Binance' },
								{ value: 'coinbase', label: 'Coinbase' },
							],
							required: true,
						},
					],
				},
			},
			{
				path: 'connectors',
				name: 'Connectors',
				component: ResourceListView,
				props: {
					apiBase,
					type: 'connectors',
					title: 'Connector catalog',
					columns: ['name', 'authType', 'rateLimit'],
				},
			},
		],
	};
}

// Default export: the apiBase-injection FACTORY. A remote module.js is built
// once, before apiBase is known — apiBase only exists at runtime, from the
// backend /apps descriptor. The forge-console-plugin loader calls this factory
// with `{ apiBase: descriptor.apiBase }` and registers the returned plugin.
//
// The factory is also tolerant of being called with no context (the loader's
// zero-arg path) — it falls back to the gateway proxy base the host uses by
// convention so the descriptor fallback in loadConsolePlugins still applies.
export default function createPlugin(ctx?: PluginContext): ForgeConsolePlugin {
	return gleipnirPlugin(ctx?.apiBase ?? '/api/proxy/gleipnir');
}
