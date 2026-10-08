<script lang="ts">
	import ListingCard from '$lib/ListingCard.svelte';
	import { ago, freshLabel, placesLabel, sourceStatus } from '$lib/format';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const watch = $derived(data.overview.watch);
	const summary = $derived(data.overview.summary);
	const source = $derived(data.overview.source);
	const title = $derived(watch?.city || watch?.name || 'Place Space');
</script>

<svelte:head>
	<title>{title} · Place Space</title>
</svelte:head>

{#if !watch}
	<section class="empty">
		<h1>Поки порожньо</h1>
		<p>Пошук ще не створено. У терміналі, з каталогу проєкту:</p>
		<p><code>go run ./cmd/placespace poll --city "Кам'янець-Подільський" --price-max 20000 --rooms-min 2</code></p>
	</section>
{:else}
	<div class="heading">
		<h1>{title}</h1>
		<p class="watching">
			<span class="dot" class:live={source?.status === 'healthy'} class:bad={source?.status === 'error'}></span>
			{watch.enabled ? 'Стежить' : 'Зупинено'}
			{#if source}
				· {sourceStatus(source.status)}
			{/if}
		</p>
	</div>
	<p class="summary">
		{placesLabel(summary.places)} · {freshLabel(summary.new)}
		{#if data.overview.run}
			· перевірено {ago(data.overview.run.finishedAt)}
		{/if}
	</p>
	<nav class="switch" aria-label="Що показувати">
		<a href="/" aria-current={data.status === 'matched' ? 'page' : undefined}>Підійшли</a>
		<a href="/?status=rejected" aria-current={data.status === 'rejected' ? 'page' : undefined}>
			Відсіяні {summary.rejected}
		</a>
		<a href="/?status=all" aria-current={data.status === 'all' ? 'page' : undefined}>Усі</a>
	</nav>
	{#if data.feed.items.length === 0}
		<p class="empty">У цьому переліку ще нічого немає.</p>
	{:else}
		<section class="grid">
			{#each data.feed.items as listing (listing.id)}
				<ListingCard {listing} />
			{/each}
		</section>
	{/if}
{/if}
