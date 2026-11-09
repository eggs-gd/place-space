import { describe, expect, it } from 'vitest';
import { coverageNote, groupAreas } from './areas';
import type { AreaOption } from './types';

function option(name: string, kind: string, sources = ['domria', 'lun']): AreaOption {
	return { key: name.toLowerCase(), name, kind, sources };
}

const options = [
	option('Осокорки', 'microdistrict'),
	option('Шевченківський', 'district'),
	option('Дарницький', 'district'),
	option('Солом’янка', 'microdistrict', ['lun'])
];

describe('groupAreas', () => {
	it('puts districts first and sorts each group', () => {
		expect(groupAreas(options, '')).toEqual([
			{ title: 'Райони', items: [options[2], options[1]] },
			{ title: 'Мікрорайони', items: [options[0], options[3]] }
		]);
	});
	it('narrows by search and drops empty groups', () => {
		expect(groupAreas(options, '  дарн ').map((group) => group.title)).toEqual(['Райони']);
	});
	it('ignores case and apostrophe style', () => {
		expect(groupAreas(options, "СОЛОМ'Я")[0].items).toEqual([options[3]]);
	});
	it('returns nothing when no name matches', () => {
		expect(groupAreas(options, 'марс')).toEqual([]);
	});
});

describe('coverageNote', () => {
	it('is empty when both sources have the area', () => {
		expect(coverageNote(options[0])).toBe('');
	});
	it('names a single source', () => {
		expect(coverageNote(options[3])).toBe('лише LUN');
		expect(coverageNote(option('Троєщина', 'microdistrict', ['domria']))).toBe('лише DOM.RIA');
	});
});
