import { loadProfiles } from '$lib/api';
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = async ({ fetch, depends }) => {
	depends('app:profiles');
	return { profiles: await loadProfiles(fetch) };
};
