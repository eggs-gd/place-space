<script lang="ts">
	import { sendJSON } from './api';
	import type { Profiles } from './types';

	let { profiles, onchosen }: { profiles: Profiles; onchosen: () => void } = $props();

	let name = $state('');
	let busy = $state(false);
	let notice = $state('');

	async function run(action: () => Promise<unknown>) {
		busy = true;
		notice = '';
		try {
			await action();
			onchosen();
		} catch (err) {
			notice = err instanceof Error ? err.message : 'Не вдалося';
		} finally {
			busy = false;
		}
	}

	const choose = (id: string) => run(() => sendJSON('POST', `/api/profiles/${encodeURIComponent(id)}/select`));
	const create = () => run(() => sendJSON('POST', '/api/profiles', { name: name.trim() }));
</script>

<section class="gate">
	<h1>Хто шукає?</h1>
	<p class="note">У кожного свій пошук. Профіль запам’ятається в цьому браузері.</p>

	{#if profiles.profiles.length > 0}
		<div class="choices">
			{#each profiles.profiles as profile (profile.id)}
				<button type="button" disabled={busy} onclick={() => choose(profile.id)}>{profile.name}</button>
			{/each}
		</div>
	{/if}

	<form
		class="editor"
		onsubmit={(event) => {
			event.preventDefault();
			create();
		}}
	>
		<label>
			{profiles.profiles.length > 0 ? 'Або новий профіль' : 'Ваше ім’я'}
			<input name="profile" bind:value={name} maxlength="40" autocomplete="off" placeholder="Наприклад, Оля" />
		</label>
		<div class="actions">
			<button type="submit" disabled={busy || !name.trim()}>Створити</button>
		</div>
		{#if notice}
			<p class="note" role="alert">{notice}</p>
		{/if}
	</form>
</section>

<style>
	.gate {
		max-width: 480px;
		margin: 48px auto 0;
	}

	.gate h1 {
		margin-bottom: 8px;
	}

	.choices {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		margin: 24px 0 8px;
	}

	.choices button {
		padding: 12px 20px;
		border: 0;
		border-radius: 999px;
		box-shadow: inset 0 0 0 1px #c7c7cc;
		background: #fff;
		color: inherit;
		font: inherit;
		cursor: pointer;
	}

	.choices button:hover:not(:disabled) {
		box-shadow: inset 0 0 0 1px #1d1d1f;
	}
</style>
