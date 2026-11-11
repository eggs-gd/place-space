import { sourceName } from './format';
import type { AreaOption } from './types';

export const MAX_AREAS = 5;
const ALL_SOURCES = 2;

export type AreaNode = { area: AreaOption; children: AreaOption[] };
export type AreaTree = { districts: AreaNode[]; others: AreaOption[] };

function searchable(value: string): string {
	return value.toLowerCase().replace(/[’ʼ`´]/g, "'");
}

function byName(a: AreaOption, b: AreaOption): number {
	return a.name.localeCompare(b.name, 'uk');
}

/**
 * Nests areas under their districts, narrowed by the search text. A matching
 * district keeps all its areas; a matching area keeps its district for context.
 */
export function areaTree(options: AreaOption[], search: string): AreaTree {
	const needle = searchable(search.trim());
	const matches = (item: AreaOption) => searchable(item.name).includes(needle);
	const districts = options.filter((item) => item.kind === 'district');
	const known = new Set(districts.map((item) => item.key));
	const nodes = districts
		.map((area) => {
			const all = options.filter((item) => item.parent === area.key && item.kind !== 'district');
			return { area, children: (matches(area) ? all : all.filter(matches)).sort(byName) };
		})
		.filter((node) => matches(node.area) || node.children.length > 0)
		.sort((a, b) => byName(a.area, b.area));
	const others = options
		.filter((item) => item.kind !== 'district' && !known.has(item.parent) && matches(item))
		.sort(byName);
	return { districts: nodes, others };
}

/** Whether the area is already part of a chosen district. */
export function isCovered(option: AreaOption, selected: string[]): boolean {
	return option.parent !== '' && selected.includes(option.parent);
}

/** The selection after ticking or clearing one area. A district absorbs its areas. */
export function toggleArea(selected: string[], option: AreaOption, on: boolean, options: AreaOption[]): string[] {
	if (!on) return selected.filter((key) => key !== option.key);
	if (selected.includes(option.key) || isCovered(option, selected)) return selected;
	const inside = new Set(options.filter((item) => item.parent === option.key).map((item) => item.key));
	return [...selected.filter((key) => !inside.has(key)), option.key];
}

/** Whether ticking the area stays within the limit. */
export function canAdd(selected: string[], option: AreaOption, options: AreaOption[]): boolean {
	return toggleArea(selected, option, true, options).length <= MAX_AREAS;
}

/** Names the one source covering an area, or nothing when both do. */
export function coverageNote(option: AreaOption): string {
	if (option.sources.length >= ALL_SOURCES || option.sources.length === 0) return '';
	return `лише ${sourceName(option.sources[0])}`;
}
