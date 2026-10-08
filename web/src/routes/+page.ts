import { getJSON } from '$lib/api';
import type { Feed, Overview } from '$lib/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const requested = url.searchParams.get('status') ?? 'matched';
	const status = ['matched', 'rejected', 'all'].includes(requested) ? requested : 'matched';
	const [overview, feed] = await Promise.all([
		getJSON<Overview>('/api/overview', fetch),
		getJSON<Feed>(`/api/listings?status=${status}`, fetch)
	]);
	return { overview, feed, status };
};
