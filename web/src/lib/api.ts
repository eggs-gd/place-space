import { dev } from '$app/environment';
import { env } from '$env/dynamic/public';
import { error } from '@sveltejs/kit';

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
	if (response.status === 404) {
		error(404, 'Оголошення не знайдено');
	}
	if (!response.ok) {
		error(response.status, 'Не вдалося завантажити дані');
	}
	return response.json() as Promise<T>;
}

export async function sendJSON<T>(method: string, path: string, body: unknown): Promise<T> {
	let response: Response;
	try {
		response = await fetch(apiOrigin() + path, {
			method,
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body)
		});
	} catch {
		throw new Error('Немає зв’язку з сервером.');
	}
	if (!response.ok) {
		const payload = (await response.json().catch(() => null)) as { error?: string } | null;
		throw new Error(payload?.error || 'Не вдалося зберегти');
	}
	return response.json() as Promise<T>;
}
