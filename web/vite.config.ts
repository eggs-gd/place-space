import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		// The dev pages reach the API through this server, so the API stays on
		// the same origin as in a build and the profile cookie works.
		proxy: {
			'/api': 'http://127.0.0.1:8080'
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
