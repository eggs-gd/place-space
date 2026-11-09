<script lang="ts">
	import { MAX_AREAS, coverageNote, groupAreas } from './areas';
	import type { AreaOption } from './types';

	let {
		options,
		selected = $bindable(),
		names,
		loading,
		error,
		onedit
	}: {
		options: AreaOption[];
		selected: string[];
		names: Record<string, string>;
		loading: boolean;
		error: string;
		onedit: () => void;
	} = $props();

	let search = $state('');

	const groups = $derived(groupAreas(options, search));
	const full = $derived(selected.length >= MAX_AREAS);

	function nameOf(key: string): string {
		return options.find((item) => item.key === key)?.name ?? names[key] ?? key;
	}

	function toggle(key: string, on: boolean) {
		selected = on ? [...selected, key] : selected.filter((item) => item !== key);
		onedit();
	}

	function keepForm(event: KeyboardEvent) {
		if (event.key === 'Enter') event.preventDefault();
	}
</script>

<fieldset class="areas">
	<legend>Райони</legend>
	{#if selected.length}
		<ul class="area-chips">
			{#each selected as key (key)}
				<li>
					<button type="button" aria-label={`Прибрати ${nameOf(key)}`} onclick={() => toggle(key, false)}>
						{nameOf(key)} <span aria-hidden="true">×</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
	{#if loading}
		<p class="note" role="status">Завантажую райони міста. Вперше це може тривати до хвилини.</p>
	{:else if error}
		<p class="note" role="alert">{error}</p>
	{:else if options.length === 0}
		<p class="note">Каталоги не поділяють це місто на райони. Шукатимемо по всьому місту.</p>
	{:else}
		<input
			type="search"
			aria-label="Знайти район"
			placeholder="Знайти район"
			autocomplete="off"
			bind:value={search}
			onkeydown={keepForm}
		/>
		<div class="area-list">
			{#each groups as group (group.title)}
				<p class="area-group">{group.title}</p>
				{#each group.items as item (item.key)}
					{@const checked = selected.includes(item.key)}
					<label class="check">
						<input
							type="checkbox"
							{checked}
							disabled={full && !checked}
							onchange={(event) => toggle(item.key, event.currentTarget.checked)}
						/>
						<span>{item.name}</span>
						{#if coverageNote(item)}
							<small>{coverageNote(item)}</small>
						{/if}
					</label>
				{/each}
			{:else}
				<p class="note">Такого району немає.</p>
			{/each}
		</div>
		<p class="note">
			{#if full}
				Можна обрати не більше {MAX_AREAS}.
			{:else if selected.length === 0}
				Нічого не обрано: шукатимемо по всьому місту.
			{:else}
				Шукатимемо лише тут.
			{/if}
		</p>
	{/if}
</fieldset>
