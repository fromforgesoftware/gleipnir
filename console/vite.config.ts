import { defineConfig, mergeConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import tailwindcss from '@tailwindcss/vite';
import { consolePluginModule } from '@fromforgesoftware/forge-console-plugin/build';

// Gleipnir console plugin build: emit a SINGLE Grafana-style SystemJS
// `module.js` (format: 'system') whose default export is the apiBase factory.
// The shared singletons (vue, vue-router, pinia, the forge kits, the
// console-plugin contract + /ui) are externalised by `consolePluginModule()` so
// the host's SystemJS import map provides the one true instance — they are NOT
// bundled.
//
// There is no Module Federation and no remoteEntry.js: the host /apps configmap
// points a moduleUri straight at the emitted dist/module.js, which is then
// pushed as an OCI artifact (see .github/workflows/release.yaml).
export default mergeConfig(
	consolePluginModule({ entry: './src/index.ts' }),
	defineConfig({
		plugins: [vue(), tailwindcss()],
	}),
);
