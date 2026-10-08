import { getJSON } from '$lib/api';
import type { Overview } from '$lib/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const overview = await getJSON<Overview>('/api/overview', fetch);
	return { overview };
};
