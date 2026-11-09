import { describe, expect, it } from 'vitest';
import { MAX_AREAS, areaTree, canAdd, coverageNote, isCovered, toggleArea } from './areas';
import type { AreaOption } from './types';

function option(name: string, kind: string, parent = '', sources = ['domria', 'lun']): AreaOption {
	return { key: name.toLowerCase(), name, kind, parent, sources };
}

const shevchenkivskyi = option('Шевченківський', 'district');
const darnytskyi = option('Дарницький', 'district');
const pozniaky = option('Позняки', 'microdistrict', 'дарницький');
const osokorky = option('Осокорки', 'microdistrict', 'дарницький');
const solomianka = option('Солом’янка', 'microdistrict', '', ['lun']);
const stray = option('Десь', 'microdistrict', 'немає такого');
const options = [pozniaky, shevchenkivskyi, osokorky, darnytskyi, solomianka, stray];

describe('areaTree', () => {
	it('nests and sorts', () => {
		expect(areaTree(options, '')).toEqual({
			districts: [
				{ area: darnytskyi, children: [osokorky, pozniaky] },
				{ area: shevchenkivskyi, children: [] }
			],
			others: [stray, solomianka]
		});
	});
	it('keeps the district of a matching area, without its other areas', () => {
		const tree = areaTree(options, 'осок');
		expect(tree.districts).toEqual([{ area: darnytskyi, children: [osokorky] }]);
		expect(tree.others).toEqual([]);
	});
	it('keeps all areas of a matching district', () => {
		expect(areaTree(options, 'дарн').districts).toEqual([
			{ area: darnytskyi, children: [osokorky, pozniaky] }
		]);
	});
	it('ignores case and apostrophe style', () => {
		expect(areaTree(options, "СОЛОМ'Я").others).toEqual([solomianka]);
	});
	it('is empty when no name matches', () => {
		expect(areaTree(options, 'марс')).toEqual({ districts: [], others: [] });
	});
});

describe('selection', () => {
	it('adds and removes', () => {
		expect(toggleArea([], osokorky, true, options)).toEqual(['осокорки']);
		expect(toggleArea(['осокорки'], osokorky, false, options)).toEqual([]);
	});
	it('lets a district absorb its areas', () => {
		expect(toggleArea(['осокорки', 'десь', 'позняки'], darnytskyi, true, options)).toEqual([
			'десь',
			'дарницький'
		]);
	});
	it('does not add an area already inside a chosen district', () => {
		expect(isCovered(osokorky, ['дарницький'])).toBe(true);
		expect(toggleArea(['дарницький'], osokorky, true, options)).toEqual(['дарницький']);
	});
	it('counts what a district would absorb against the limit', () => {
		const full = ['осокорки', 'позняки', 'а', 'б', 'в'];
		expect(full).toHaveLength(MAX_AREAS);
		expect(canAdd(full, solomianka, options)).toBe(false);
		expect(canAdd(full, darnytskyi, options)).toBe(true);
	});
});

describe('coverageNote', () => {
	it('is empty when both sources have the area', () => {
		expect(coverageNote(osokorky)).toBe('');
	});
	it('names a single source', () => {
		expect(coverageNote(solomianka)).toBe('лише LUN');
		expect(coverageNote(option('Троєщина', 'microdistrict', '', ['domria']))).toBe('лише DOM.RIA');
	});
});
