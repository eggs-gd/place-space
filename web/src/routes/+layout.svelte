<script lang="ts">
	import { browser } from '$app/environment';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { sendJSON } from '$lib/api';
	import ProfileGate from '$lib/ProfileGate.svelte';
	import type { Snippet } from 'svelte';
	import '../app.css';
	import type { LayoutData } from './$types';

	let { children, data }: { children: Snippet; data: LayoutData } = $props();

	const profiles = $derived(data.profiles);
	const gate = $derived(profiles.enabled && !profiles.current);

	async function leave() {
		await sendJSON('DELETE', '/api/profiles/current').catch(() => undefined);
		await invalidateAll();
	}

	const path = $derived(page.url.pathname);
	const places = $derived(path === '/' || path.startsWith('/listings'));
	const criteria = $derived(path.startsWith('/watch'));

	$effect(() => {
		if (!browser) return;
		const refresh = () => {
			if (document.visibilityState === 'visible' && !gate) invalidateAll();
		};
		const timer = setInterval(refresh, 30_000);
		document.addEventListener('visibilitychange', refresh);
		return () => {
			clearInterval(timer);
			document.removeEventListener('visibilitychange', refresh);
		};
	});
</script>

<svelte:head>
	<title>Place Space</title>
	<link rel="manifest" href="/manifest.webmanifest" />
	<meta name="theme-color" content="#f5f5f7" />
</svelte:head>

<header class="top">
	<div class="shell top-inner">
		<a class="brand" href="/">Place Space</a>
		{#if !gate}
			<nav class="nav" aria-label="Розділи">
				<a href="/" aria-current={places ? 'page' : undefined}>Місця</a>
				<a href="/watch" aria-current={criteria ? 'page' : undefined}>Критерії</a>
				{#if profiles.current}
					<button type="button" class="who" title="Змінити профіль" onclick={leave}>{profiles.current.name}</button>
				{/if}
			</nav>
		{/if}
	</div>
</header>

<main class="page">
	<div class="shell">
		{#if gate}
			<ProfileGate {profiles} onchosen={() => invalidateAll()} />
		{:else}
			{@render children()}
		{/if}
	</div>
</main>
