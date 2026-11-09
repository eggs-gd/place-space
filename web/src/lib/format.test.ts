import { describe, expect, it } from 'vitest';
import {
	ago,
	area,
	criteria,
	facts,
	freshLabel,
	intervalLabel,
	money,
	placeLine,
	placesLabel,
	reasonText,
	roomsLabel,
	sourceName,
	sourceStatus
} from './format';
import type { Reason, Watch } from './types';

// Intl separates thousands with a no-break space.
const plain = (text: string) => text.replace(/\s/g, ' ');

function reason(field: string, message: string, passed = false): Reason {
	return { field, message, passed };
}

function watch(overrides: Partial<Watch>): Watch {
	return {
		id: 'w',
		name: 'Київ',
		enabled: true,
		city: 'Київ',
		deal: 'rent',
		properties: ['apartment'],
		priceMin: null,
		priceMax: null,
		currency: '',
		roomsMin: null,
		areaMin: null,
		pollIntervalSeconds: 600,
		...overrides
	} as Watch;
}

describe('money', () => {
	it('uses the hryvnia sign by default', () => {
		expect(plain(money(18000))).toBe('18 000 ₴');
		expect(plain(money(18000, ''))).toBe('18 000 ₴');
	});
	it('keeps a foreign price as published', () => {
		expect(plain(money(1900, 'USD'))).toBe('1 900 USD');
	});
	it('says when the price is unknown', () => {
		expect(money(null)).toBe('ціна невідома');
	});
});

describe('plural labels', () => {
	it.each([
		[1, '1 кімната'],
		[2, '2 кімнати'],
		[5, '5 кімнат'],
		[11, '11 кімнат'],
		[21, '21 кімната'],
		[22, '22 кімнати']
	])('rooms %i', (count, label) => {
		expect(roomsLabel(count)).toBe(label);
	});
	it('counts places and new listings', () => {
		expect(placesLabel(3)).toBe('3 місця');
		expect(freshLabel(1)).toBe('1 нове');
	});
});

describe('facts', () => {
	const base = { rooms: null, area: null, floor: null, totalFloors: null };
	it('joins what is known', () => {
		expect(facts({ rooms: 2, area: 62.5, floor: 3, totalFloors: 9 })).toBe(
			`2 кімнати · ${area(62.5)} · 3/9`
		);
	});
	it('handles a floor without a total and an empty listing', () => {
		expect(facts({ ...base, floor: 4 })).toBe('4 поверх');
		expect(facts(base)).toBe('');
	});
});

describe('ago', () => {
	const now = Date.parse('2026-11-09T12:00:00Z');
	it.each([
		['2026-11-09T11:59:50Z', 'щойно'],
		['2026-11-09T11:30:00Z', '30 хв тому'],
		['2026-11-09T09:00:00Z', '3 години тому'],
		['2026-11-07T12:00:00Z', '2 дні тому']
	])('%s', (iso, label) => {
		expect(ago(iso, now)).toBe(label);
	});
	it('returns nothing for a bad date', () => {
		expect(ago('nope', now)).toBe('');
	});
});

describe('source labels', () => {
	it('names known sources', () => {
		expect(sourceName('lun')).toBe('LUN');
		expect(sourceName('domria')).toBe('DOM.RIA');
		expect(sourceName('')).toBe('Джерело');
	});
	it('describes status', () => {
		expect(sourceStatus('healthy')).toBe('Працює');
		expect(sourceStatus('error')).toBe('Помилка');
		expect(sourceStatus('unknown')).toBe('Ще немає перевірки');
	});
});

describe('intervalLabel', () => {
	it.each([
		[600, '10 хв'],
		[3600, '1 година'],
		[7200, '2 години'],
		[45, '45 с']
	])('%i', (seconds, label) => {
		expect(intervalLabel(seconds)).toBe(label);
	});
});

describe('criteria', () => {
	it('lists every bound', () => {
		const lines = criteria(
			watch({ priceMax: 20000, priceMin: 5000, roomsMin: 2, areaMin: 40 })
		).map(plain);
		expect(lines).toEqual([
			'Оренда',
			'≤ 20 000 ₴',
			'≥ 5 000 ₴',
			'Квартира',
			'2+ кімнати',
			`від ${plain(area(40))}`
		]);
	});
	it('names both property types', () => {
		expect(criteria(watch({ properties: ['apartment', 'house'] }))).toContain(
			'Квартира або будинок'
		);
	});
});

describe('reasonText', () => {
	it('explains a price over the limit', () => {
		expect(plain(reasonText(reason('price', 'price: 22500 > 20000')))).toBe(
			'22 500 ₴ вище за 20 000 ₴'
		);
	});
	it('explains a price under the minimum', () => {
		expect(plain(reasonText(reason('price', 'price: 3000 < 5000')))).toBe('3 000 ₴ нижче за 5 000 ₴');
	});
	it('explains rooms and area', () => {
		expect(reasonText(reason('rooms', 'rooms: 1 < 2'))).toBe('1 кімната замість 2+');
		expect(reasonText(reason('area', 'area: 30 < 40'))).toBe(`${area(30)} менше за ${area(40)}`);
	});
	it('explains unknown facts', () => {
		expect(reasonText(reason('price', 'price: unknown'))).toBe('ціна невідома');
		expect(reasonText(reason('rooms', 'rooms: unknown'))).toBe('кількість кімнат невідома');
		expect(reasonText(reason('area', 'area: unknown'))).toBe('площа невідома');
	});
	it('distinguishes a missing rate from a missing currency', () => {
		expect(reasonText(reason('currency', 'currency: PLN != UAH'))).toBe('немає курсу PLN до UAH');
		expect(reasonText(reason('currency', 'currency: unknown != UAH'))).toBe('валюта невідома');
	});
	it('falls back to the raw message', () => {
		expect(reasonText(reason('other', 'something else'))).toBe('something else');
	});
});

describe('placeLine', () => {
	it('prefers the address, then the title, then the location', () => {
		expect(placeLine({ address: 'а', title: 'б', location: 'в' })).toBe('а');
		expect(placeLine({ address: '', title: 'б', location: 'в' })).toBe('б');
		expect(placeLine({ address: '', title: '', location: 'в' })).toBe('в');
	});
});
