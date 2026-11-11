import { getJSON } from '$lib/api';
import type { Overview, Settings } from '$lib/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [overview, settings] = await Promise.all([
		getJSON<Overview>('/api/overview', fetch),
		getJSON<Settings>('/api/settings', fetch)
	]);
	return { overview, settings };
};
