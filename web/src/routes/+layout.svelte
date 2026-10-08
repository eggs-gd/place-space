<script lang="ts">
	import { browser } from '$app/environment';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import type { Snippet } from 'svelte';
	import '../app.css';

	let { children }: { children: Snippet } = $props();

	const path = $derived(page.url.pathname);
	const places = $derived(path === '/' || path.startsWith('/listings'));
	const criteria = $derived(path.startsWith('/watch'));

	$effect(() => {
		if (!browser) return;
		const refresh = () => {
			if (document.visibilityState === 'visible') invalidateAll();
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
		<nav class="nav" aria-label="Розділи">
			<a href="/" aria-current={places ? 'page' : undefined}>Місця</a>
			<a href="/watch" aria-current={criteria ? 'page' : undefined}>Критерії</a>
		</nav>
	</div>
</header>

<main class="page">
	<div class="shell">
		{@render children()}
	</div>
</main>
