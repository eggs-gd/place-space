import { getJSON } from '$lib/api';
import type { ListingDetail } from '$lib/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const listing = await getJSON<ListingDetail>(`/api/listings/${params.id}`, fetch);
	return { listing };
};
