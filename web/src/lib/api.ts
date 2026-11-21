import { dev } from '$app/environment';
import { env } from '$env/dynamic/public';
import { error } from '@sveltejs/kit';
import type { AreaOption, Profiles } from './types';

export function apiOrigin(): string {
	if (env.PUBLIC_API_ORIGIN) return env.PUBLIC_API_ORIGIN.replace(/\/$/, '');
	if (dev) return 'http://127.0.0.1:8080';
	return '';
}

export async function getJSON<T>(path: string, fetchFn: typeof fetch): Promise<T> {
	let response: Response;
	try {
		response = await fetchFn(apiOrigin() + path);
	} catch {
		error(503, 'Немає зв’язку з сервером.');
	}
	if (response.status === 401) {
		error(401, 'Оберіть профіль');
	}
	if (response.status === 404) {
		error(404, 'Оголошення не знайдено');
	}
	if (!response.ok) {
		error(response.status, 'Не вдалося завантажити дані');
	}
	return response.json() as Promise<T>;
}

export async function loadAreas(city: string): Promise<AreaOption[]> {
	let response: Response;
	try {
		response = await fetch(`${apiOrigin()}/api/areas?city=${encodeURIComponent(city)}`);
	} catch {
		throw new Error('Немає зв’язку з сервером.');
	}
	const payload = (await response.json().catch(() => null)) as {
		areas?: AreaOption[];
		error?: string;
	} | null;
	if (!response.ok) throw new Error(payload?.error || 'Не вдалося завантажити райони');
	return payload?.areas ?? [];
}

export async function sendJSON<T>(method: string, path: string, body?: unknown): Promise<T> {
	let response: Response;
	try {
		response = await fetch(apiOrigin() + path, {
			method,
			headers: { 'Content-Type': 'application/json' },
			body: body === undefined ? undefined : JSON.stringify(body)
		});
	} catch {
		throw new Error('Немає зв’язку з сервером.');
	}
	if (!response.ok) {
		const payload = (await response.json().catch(() => null)) as { error?: string } | null;
		throw new Error(payload?.error || 'Не вдалося зберегти');
	}
	if (response.status === 204) return undefined as T;
	return response.json() as Promise<T>;
}

const noProfiles: Profiles = { enabled: false, current: null, profiles: [] };

// loadProfiles never fails: without an answer the interface behaves as a
// single-person install, and the pages report the missing server themselves.
export async function loadProfiles(fetchFn: typeof fetch): Promise<Profiles> {
	try {
		const response = await fetchFn(apiOrigin() + '/api/profiles');
		if (!response.ok) return noProfiles;
		return (await response.json()) as Profiles;
	} catch {
		return noProfiles;
	}
}
