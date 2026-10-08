import type { Reason, Watch } from './types';

const priceFormat = new Intl.NumberFormat('uk-UA', { maximumFractionDigits: 0 });
const areaFormat = new Intl.NumberFormat('uk-UA', { maximumFractionDigits: 1 });
const whenFormat = new Intl.DateTimeFormat('uk-UA', {
	day: 'numeric',
	month: 'long',
	hour: '2-digit',
	minute: '2-digit'
});

export function money(amount: number | null, currency = 'UAH'): string {
	if (amount == null) return 'ціна невідома';
	const formatted = priceFormat.format(amount);
	if (!currency || currency === 'UAH') return `${formatted} ₴`;
	return `${formatted} ${currency}`;
}

export function area(value: number | null): string {
	if (value == null) return '';
	return `${areaFormat.format(value)} м²`;
}

export function roomsLabel(count: number): string {
	return `${count} ${plural(count, 'кімната', 'кімнати', 'кімнат')}`;
}

export function placesLabel(count: number): string {
	return `${count} ${plural(count, 'місце', 'місця', 'місць')}`;
}

export function freshLabel(count: number): string {
	return `${count} ${plural(count, 'нове', 'нові', 'нових')}`;
}

export function facts(listing: {
	rooms: number | null;
	area: number | null;
	floor: number | null;
	totalFloors: number | null;
}): string {
	const parts: string[] = [];
	if (listing.rooms != null) parts.push(roomsLabel(listing.rooms));
	if (listing.area != null) parts.push(area(listing.area));
	if (listing.floor != null && listing.totalFloors != null) {
		parts.push(`${listing.floor}/${listing.totalFloors}`);
	} else if (listing.floor != null) {
		parts.push(`${listing.floor} поверх`);
	}
	return parts.join(' · ');
}

export function ago(iso: string, now = Date.now()): string {
	const minutes = Math.round((now - new Date(iso).getTime()) / 60000);
	if (!Number.isFinite(minutes)) return '';
	if (minutes < 1) return 'щойно';
	if (minutes < 60) return `${minutes} хв тому`;
	const hours = Math.round(minutes / 60);
	if (hours < 24) return `${hours} ${plural(hours, 'годину', 'години', 'годин')} тому`;
	const days = Math.round(hours / 24);
	return `${days} ${plural(days, 'день', 'дні', 'днів')} тому`;
}

export function when(iso: string): string {
	return whenFormat.format(new Date(iso));
}

export function sourceName(type: string): string {
	if (type === 'lun') return 'LUN';
	return type || 'Джерело';
}

export function sourceStatus(status: string): string {
	if (status === 'healthy') return 'Працює';
	if (status === 'error') return 'Помилка';
	return 'Ще немає перевірки';
}

export function intervalLabel(seconds: number): string {
	if (seconds > 0 && seconds % 3600 === 0) {
		const hours = seconds / 3600;
		return `${hours} ${plural(hours, 'година', 'години', 'годин')}`;
	}
	if (seconds > 0 && seconds % 60 === 0) {
		const minutes = seconds / 60;
		return `${minutes} хв`;
	}
	return `${seconds} с`;
}

export function criteria(watch: Watch): string[] {
	const lines: string[] = [];
	if (watch.deal === 'rent') lines.push('Оренда');
	else if (watch.deal) lines.push(watch.deal);
	const currency = watch.currency || 'UAH';
	if (watch.priceMax != null) lines.push(`≤ ${money(watch.priceMax, currency)}`);
	if (watch.priceMin != null) lines.push(`≥ ${money(watch.priceMin, currency)}`);
	const properties = new Set(watch.properties);
	if (properties.has('apartment') && properties.has('house')) lines.push('Квартира або будинок');
	else if (properties.has('house')) lines.push('Будинок');
	else if (properties.has('apartment') || properties.size === 0) lines.push('Квартира');
	if (watch.roomsMin != null) lines.push(`${watch.roomsMin}+ ${plural(watch.roomsMin, 'кімната', 'кімнати', 'кімнат')}`);
	if (watch.areaMin != null) lines.push(`від ${area(watch.areaMin)}`);
	return lines;
}

export function reasonText(reason: Reason): string {
	const compared = reason.message.match(/^(price|rooms|area): ([0-9.]+) (<=|>=|<|>) ([0-9.]+)$/);
	if (compared) {
		const field = compared[1];
		const value = Number(compared[2]);
		const op = compared[3];
		const limit = Number(compared[4]);
		if (field === 'price') {
			if (op === '>' || op === '>=') return `${money(value)} вище за ${money(limit)}`;
			if (op === '<') return `${money(value)} нижче за ${money(limit)}`;
			return `${money(value)} у межах до ${money(limit)}`;
		}
		if (field === 'rooms') {
			if (op === '<' || op === '<=') return `${roomsLabel(value)} замість ${limit}+`;
			return `${roomsLabel(value)}, потрібно від ${limit}`;
		}
		if (op === '<' || op === '<=') return `${area(value)} менше за ${area(limit)}`;
		return `${area(value)}, потрібно від ${area(limit)}`;
	}
	if (reason.message.endsWith(': unknown')) {
		if (reason.field === 'price') return 'ціна невідома';
		if (reason.field === 'rooms') return 'кількість кімнат невідома';
		if (reason.field === 'area') return 'площа невідома';
	}
	if (reason.message.startsWith('currency:')) return `валюта не ${reason.message.split('!=')[1]?.trim() ?? ''}`.trim();
	return reason.message;
}

export function placeLine(listing: { address: string; title: string; location: string }): string {
	return listing.address || listing.title || listing.location;
}

function plural(count: number, one: string, few: string, many: string): string {
	const mod10 = count % 10;
	const mod100 = count % 100;
	if (mod10 === 1 && mod100 !== 11) return one;
	if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
	return many;
}
