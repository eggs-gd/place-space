<script lang="ts">
	import { facts, money, placeLine, reasonText, sourceName, ago } from './format';
	import type { Card } from './types';

	let { listing }: { listing: Card } = $props();
	let imageFailed = $state(false);
	const failedReasons = $derived(listing.reasons.filter((reason) => !reason.passed));
	const seenAt = $derived(listing.publishedAt || listing.lastSeenAt || listing.firstSeenAt);
</script>

<a class="card" href="/listings/{listing.id}">
	<div class="photo">
		{#if listing.image && !imageFailed}
			<img src={listing.image} alt={listing.title || placeLine(listing)} onerror={() => (imageFailed = true)} />
		{:else}
			<div class="fallback" aria-hidden="true"></div>
		{/if}
		{#if listing.isNew}
			<span class="badge">Нове</span>
		{/if}
	</div>
	<div class="copy">
		<p class="price">{money(listing.priceAmount, listing.currency)}</p>
		{#if facts(listing)}
			<p class="meta">{facts(listing)}</p>
		{/if}
		<p class="place">{placeLine(listing)}</p>
		{#if listing.location && listing.address}
			<p class="city">{listing.location}</p>
		{/if}
		{#each failedReasons as reason (reason.field + reason.message)}
			<p class="reason">{reasonText(reason)}</p>
		{/each}
		<p class="origin">{sourceName(listing.source)} · {ago(seenAt)}</p>
	</div>
</a>

<style>
	.card {
		display: block;
		color: inherit;
		text-decoration: none;
	}

	.card:hover .photo img,
	.card:focus-visible .photo img {
		transform: scale(1.02);
	}

	.photo {
		position: relative;
		aspect-ratio: 3 / 2;
		overflow: hidden;
		border-radius: 20px;
		background: linear-gradient(165deg, #ececef, #dddde3);
	}

	.photo img,
	.fallback {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
		transition: transform 200ms ease;
	}

	.badge {
		position: absolute;
		top: 12px;
		left: 12px;
		padding: 4px 10px;
		border-radius: 999px;
		background: #1d1d1f;
		color: white;
		font-size: 13px;
		line-height: 1.3;
	}

	.copy {
		padding: 14px 4px 0;
	}

	.price {
		margin: 0;
		font-size: 1.65rem;
		font-weight: 600;
		letter-spacing: -0.03em;
		line-height: 1.1;
	}

	.meta,
	.place,
	.city,
	.origin,
	.reason {
		margin: 6px 0 0;
		line-height: 1.35;
	}

	.meta,
	.city,
	.origin {
		color: #515154;
	}

	.place {
		font-size: 1.02rem;
	}

	.reason {
		color: #9a3412;
	}

	.origin {
		font-size: 0.92rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.photo img {
			transition: none;
		}
	}
</style>
