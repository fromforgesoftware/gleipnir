/// <reference types="vite/client" />

// SFC shim so vue-tsc and editors know `*.vue` imports resolve to a Vue
// component.
declare module '*.vue' {
	import type { DefineComponent } from 'vue';
	const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>;
	export default component;
}
