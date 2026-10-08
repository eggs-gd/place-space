<script lang="ts">
	import {
		ago,
		facts,
		money,
		placeLine,
		reasonText,
		sourceName,
		when
	} from '$lib/format';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let selected = $state(0);

	const listing = $derived(data.listing);
	const images = $derived(listing.images.length ? listing.images : listing.image ? [listing.image] : []);
	const current = $derived(images[selected] ?? '');

	$effect(() => {
		listing.id;
		selected = 0;
	});
</script>

<svelte:head>
	<title>{listing.title || placeLine(listing)} · Place Space</title>
</svelte:head>

<a class="back" href="/">Місця</a>

<article class="detail">
	<div class="gallery">
		{#if current}
			<img class="hero-photo" src={current} alt={listing.title || placeLine(listing)} />
		{:else}
			<div class="hero-photo" role="img" aria-label="Фото немає"></div>
		{/if}
		{#if images.length > 1}
			<div class="thumbs">
				{#each images as image, index (image + index)}
					<button type="button" aria-label="Фото {index + 1}" aria-current={index === selected ? 'true' : undefined} onclick={() => (selected = index)}>
						<img src={image} alt="" />
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<div>
		<p class="price">{money(listing.priceAmount, listing.currency)}</p>
		<h1>{listing.title || placeLine(listing)}</h1>
		{#if facts(listing)}
			<p class="lead">{facts(listing)}</p>
		{/if}
		{#if listing.address}
			<p class="lead">{listing.address}{listing.location ? `, ${listing.location}` : ''}</p>
		{:else if listing.location}
			<p class="lead">{listing.location}</p>
		{/if}
		{#if listing.description}
			<p class="description">{listing.description}</p>
		{/if}
		<p>
			<a class="open" href={listing.url} target="_blank" rel="noreferrer">Відкрити оголошення</a>
		</p>

		<section class="panel">
			<h2>Чому так вирішено</h2>
			{#if listing.reasons.length === 0}
				<p class="note">Пояснення ще немає.</p>
			{:else}
				<ul class="facts">
					{#each listing.reasons as reason (reason.field + reason.message)}
						<li>
							<span>{reason.passed ? 'Пройшло' : 'Відсіяно'}</span>
							<span>{reasonText(reason)}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="panel">
			<h2>Коли бачили</h2>
			<ul class="facts">
				<li><span>Вперше</span><span>{when(listing.firstSeenAt)}</span></li>
				<li><span>Востаннє</span><span>{when(listing.lastSeenAt)} · {ago(listing.lastSeenAt)}</span></li>
				{#if listing.publishedAt}
					<li><span>Опубліковано</span><span>{when(listing.publishedAt)}</span></li>
				{/if}
			</ul>
		</section>

		<section class="panel">
			<h2>Ціна</h2>
			{#if listing.history.length === 0}
				<p class="note">Історії ціни ще немає.</p>
			{:else}
				<ul class="timeline">
					{#each listing.history as point (point.seenAt + point.priceAmount)}
						<li>
							<span>{when(point.seenAt)}</span>
							<span>{money(point.priceAmount, point.currency)}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		{#if listing.duplicates.length > 0}
			<section class="panel">
				<h2>Той самий URL</h2>
				<ul class="facts">
					{#each listing.duplicates as other (other.id)}
						<li>
							<a href="/listings/{other.id}">{other.title || sourceName(other.source)}</a>
							<span>{money(other.priceAmount, other.currency)}</span>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	</div>
</article>
