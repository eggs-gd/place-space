import { sourceName } from './format';
import type { AreaOption } from './types';

export const MAX_AREAS = 5;
const ALL_SOURCES = 2;

export type AreaGroup = { title: string; items: AreaOption[] };

function searchable(value: string): string {
	return value.toLowerCase().replace(/[’ʼ`´]/g, "'");
}

/** Splits options into districts and smaller areas, narrowed by the search text. */
export function groupAreas(options: AreaOption[], search: string): AreaGroup[] {
	const needle = searchable(search.trim());
	const matching = options
		.filter((item) => searchable(item.name).includes(needle))
		.sort((a, b) => a.name.localeCompare(b.name, 'uk'));
	const groups = [
		{ title: 'Райони', items: matching.filter((item) => item.kind === 'district') },
		{ title: 'Мікрорайони', items: matching.filter((item) => item.kind !== 'district') }
	];
	return groups.filter((group) => group.items.length > 0);
}

/** Names the one source covering an area, or nothing when both do. */
export function coverageNote(option: AreaOption): string {
	if (option.sources.length >= ALL_SOURCES || option.sources.length === 0) return '';
	return `лише ${sourceName(option.sources[0])}`;
}
