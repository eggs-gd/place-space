<script lang="ts">
	import { MAX_AREAS, areaTree, canAdd, coverageNote, isCovered, toggleArea } from './areas';
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
	let opened = $state<Record<string, boolean>>({});

	const tree = $derived(areaTree(options, search));
	const searching = $derived(search.trim() !== '');
	const empty = $derived(tree.districts.length === 0 && tree.others.length === 0);

	function nameOf(key: string): string {
		return options.find((item) => item.key === key)?.name ?? names[key] ?? key;
	}

	function isOpen(key: string, children: AreaOption[]): boolean {
		return searching || (opened[key] ?? children.some((item) => selected.includes(item.key)));
	}

	function toggle(item: AreaOption, on: boolean) {
		selected = toggleArea(selected, item, on, options);
		onedit();
	}

	function remove(key: string) {
		selected = selected.filter((item) => item !== key);
		onedit();
	}

	function keepForm(event: KeyboardEvent) {
		if (event.key === 'Enter') event.preventDefault();
	}
</script>

{#snippet row(item: AreaOption)}
	{@const covered = isCovered(item, selected)}
	{@const checked = covered || selected.includes(item.key)}
	<label class="check">
		<input
			type="checkbox"
			{checked}
			disabled={covered || (!checked && !canAdd(selected, item, options))}
			onchange={(event) => toggle(item, event.currentTarget.checked)}
		/>
		<span>{item.name}</span>
		{#if covered}
			<small>входить у район</small>
		{:else if coverageNote(item)}
			<small>{coverageNote(item)}</small>
		{/if}
	</label>
{/snippet}

<fieldset class="areas">
	<legend>Райони</legend>
	{#if selected.length}
		<ul class="area-chips">
			{#each selected as key (key)}
				<li>
					<button type="button" aria-label={`Прибрати ${nameOf(key)}`} onclick={() => remove(key)}>
						{nameOf(key)} <span aria-hidden="true">×</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
	{#if loading}
		<p class="note" role="status">Завантажую райони міста. Вперше це може тривати кілька хвилин.</p>
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
			{#each tree.districts as node (node.area.key)}
				{@const open = isOpen(node.area.key, node.children)}
				<div class="area-row">
					{#if node.children.length}
						<button
							type="button"
							class="area-toggle"
							class:open
							aria-expanded={open}
							aria-label={`${open ? 'Згорнути' : 'Розгорнути'} ${node.area.name}`}
							onclick={() => (opened[node.area.key] = !open)}
						>
							<span aria-hidden="true">›</span>
						</button>
					{:else}
						<span class="area-toggle-space"></span>
					{/if}
					{@render row(node.area)}
				</div>
				{#if open}
					<div class="area-children">
						{#each node.children as child (child.key)}
							{@render row(child)}
						{/each}
					</div>
				{/if}
			{/each}
			{#if tree.others.length}
				<p class="area-group">{tree.districts.length ? 'Район не визначено' : 'Мікрорайони'}</p>
				{#each tree.others as item (item.key)}
					{@render row(item)}
				{/each}
			{/if}
			{#if empty}
				<p class="note">Такого району немає.</p>
			{/if}
		</div>
		<p class="note">
			{#if selected.length >= MAX_AREAS}
				Можна обрати не більше {MAX_AREAS}.
			{:else if selected.length === 0}
				Нічого не обрано: шукатимемо по всьому місту.
			{:else}
				Шукатимемо лише тут.
			{/if}
		</p>
	{/if}
</fieldset>
