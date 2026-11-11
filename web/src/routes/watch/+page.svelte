<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import AreaPicker from '$lib/AreaPicker.svelte';
	import { loadAreas, sendJSON } from '$lib/api';
	import { ago, distanceLabel, sourceName, sourceStatus } from '$lib/format';
	import type { AreaOption, Watch } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const watch = $derived(data.overview.watch);
	const sources = $derived(
		data.overview.sources?.length
			? data.overview.sources
			: data.overview.source
				? [data.overview.source]
				: []
	);
	const sourceState = $derived(
		sources.some((item) => item.status === 'error')
			? 'error'
			: sources.some((item) => item.status === 'healthy')
				? 'healthy'
				: ''
	);
	const run = $derived(data.overview.run);

	let editing = $state(false);
	let formKey = $state('');
	let city = $state('');
	let priceMax = $state('');
	let priceMin = $state('');
	let roomsMin = $state('');
	let areaMin = $state('');
	let property = $state('apartment');
	let enabled = $state(true);
	let saving = $state(false);
	let notice = $state('');
	let areas = $state<string[]>([]);
	let areaOptions = $state<AreaOption[]>([]);
	let areasLoading = $state(false);
	let areasError = $state('');
	let loadedCity = '';
	let areasRequest = 0;
	let nearName = $state('');
	let nearRadius = $state('1000');
	let keyInput = $state('');
	let keySaving = $state(false);
	let keyNotice = $state('');

	const radii = [300, 500, 1000, 2000, 3000, 5000];
	const keyState = $derived(data.settings.googleKey);

	const areaNames = $derived(Object.fromEntries((watch?.areas ?? []).map((item) => [item.key, item.name])));

	async function refreshAreas(name: string) {
		const trimmed = name.trim();
		if (trimmed === loadedCity) return;
		loadedCity = trimmed;
		const request = ++areasRequest;
		areaOptions = [];
		areasError = '';
		if (!trimmed) {
			areasLoading = false;
			return;
		}
		areasLoading = true;
		try {
			const options = await loadAreas(trimmed);
			if (request === areasRequest) areaOptions = options;
		} catch (err) {
			if (request === areasRequest) {
				areasError = err instanceof Error ? err.message : 'Не вдалося завантажити райони';
				loadedCity = '';
			}
		} finally {
			if (request === areasRequest) areasLoading = false;
		}
	}

	function cityChanged() {
		if (city.trim() !== loadedCity) areas = [];
		refreshAreas(city);
	}

	const nextCheck = $derived.by(() => {
		if (!watch || !run) return '';
		const next = new Date(run.finishedAt).getTime() + watch.pollIntervalSeconds * 1000;
		const wait = next - Date.now();
		if (wait <= 0) return 'перевірка зараз';
		const minutes = Math.max(1, Math.round(wait / 60000));
		return `наступна перевірка ~${minutes} хв`;
	});

	$effect(() => {
		if (editing) return;
		const next = watch
			? [
					watch.id,
					watch.city,
					watch.priceMin,
					watch.priceMax,
					watch.roomsMin,
					watch.areaMin,
					watch.properties.join(','),
					watch.areas.map((item) => item.key).join(','),
					watch.near ? `${watch.near.name}@${watch.near.radiusMeters}` : '',
					watch.enabled
				].join('|')
			: 'new';
		if (next === formKey) return;
		formKey = next;
		fill(watch);
	});

	function fill(current: Watch | null) {
		if (!current) {
			city = '';
			priceMax = '';
			priceMin = '';
			roomsMin = '';
			areaMin = '';
			property = 'apartment';
			enabled = true;
			areas = [];
			nearName = '';
			nearRadius = '1000';
			return;
		}
		nearName = current.near?.name ?? '';
		nearRadius = String(current.near?.radiusMeters ?? 1000);
		city = current.city;
		areas = current.areas.map((item) => item.key);
		refreshAreas(current.city);
		priceMax = current.priceMax == null ? '' : String(current.priceMax);
		priceMin = current.priceMin == null ? '' : String(current.priceMin);
		roomsMin = current.roomsMin == null ? '' : String(current.roomsMin);
		areaMin = current.areaMin == null ? '' : String(current.areaMin);
		property =
			current.properties.includes('apartment') && current.properties.includes('house')
				? 'both'
				: current.properties.includes('house')
					? 'house'
					: 'apartment';
		enabled = current.enabled;
	}

	function blankNumber(raw: string | number | null): number | null {
		if (raw == null) return null;
		if (typeof raw === 'number') return Number.isFinite(raw) ? raw : null;
		const trimmed = raw.trim();
		if (!trimmed) return null;
		const value = Number(trimmed);
		if (!Number.isFinite(value)) throw new Error('Вкажіть число');
		return value;
	}

	async function storeKey(value: string) {
		keySaving = true;
		keyNotice = '';
		try {
			await sendJSON('PUT', '/api/settings', { googleKey: value });
			keyInput = '';
			keyNotice = value ? 'Ключ перевірено і збережено.' : 'Ключ прибрано.';
			await invalidateAll();
		} catch (err) {
			keyNotice = err instanceof Error ? err.message : 'Не вдалося зберегти ключ';
		} finally {
			keySaving = false;
		}
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		notice = '';
		try {
			const body = {
				city: String(city).trim(),
				priceMin: blankNumber(priceMin),
				priceMax: blankNumber(priceMax),
				roomsMin: blankNumber(roomsMin),
				areaMin: blankNumber(areaMin),
				properties: property === 'both' ? ['apartment', 'house'] : [property],
				areas,
				near: nearName.trim() ? { name: nearName.trim(), radiusMeters: Number(nearRadius) } : null,
				enabled
			};
			if (watch) {
				await sendJSON('PATCH', `/api/watches/${watch.id}`, body);
				notice = enabled ? 'Збережено. Перевірка вже йде.' : 'Збережено. Стеження зупинено.';
			} else {
				await sendJSON('POST', '/api/watches', body);
				notice = 'Пошук створено. Перевірка вже йде.';
			}
			editing = false;
			await invalidateAll();
		} catch (err) {
			notice = err instanceof Error ? err.message : 'Не вдалося зберегти';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Критерії · Place Space</title>
</svelte:head>

<div class="heading">
	<h1>{watch?.city || watch?.name || 'Новий пошук'}</h1>
	{#if watch}
		<p class="watching">
			<span class="dot" class:live={watch.enabled && sourceState === 'healthy'} class:bad={sourceState === 'error'}></span>
			{watch.enabled ? 'Стежить' : 'Зупинено'}
		</p>
	{/if}
</div>

<form class="editor" oninput={() => (editing = true)} onsubmit={save}>
	<label>
		Місто
		<input name="city" bind:value={city} onchange={cityChanged} required autocomplete="off" />
	</label>
	{#if city.trim()}
		<AreaPicker
			options={areaOptions}
			bind:selected={areas}
			names={areaNames}
			loading={areasLoading}
			error={areasError}
			onedit={() => (editing = true)}
		/>
	{/if}
	<div class="near">
		<label>
			Біля адреси
			<input name="near" bind:value={nearName} placeholder="вулиця, будинок чи місце" autocomplete="off" />
		</label>
		<label>
			Радіус
			<select name="nearRadius" bind:value={nearRadius}>
				{#each radii as radius (radius)}
					<option value={String(radius)}>{distanceLabel(radius)}</option>
				{/each}
			</select>
		</label>
	</div>
	{#if nearName.trim()}
		<p class="note">
			Біля адреси шукає лише Otodim: LUN і DOM.RIA радіуса не мають і без районів цей пошук пропускають.
			{#if !keyState.set}Потрібен ключ Google, його можна вказати внизу сторінки.{/if}
		</p>
	{/if}
	<label>
		Ціна до, ₴
		<input name="priceMax" type="number" min="0" step="1" bind:value={priceMax} placeholder="без межі" />
	</label>
	<label>
		Ціна від, ₴
		<input name="priceMin" type="number" min="0" step="1" bind:value={priceMin} placeholder="без межі" />
	</label>
	<label>
		Кімнат від
		<input name="roomsMin" type="number" min="1" step="1" bind:value={roomsMin} placeholder="без межі" />
	</label>
	<label>
		Площа від, м²
		<input name="areaMin" type="number" min="0" step="0.1" bind:value={areaMin} placeholder="без межі" />
	</label>
	<label>
		Житло
		<select name="property" bind:value={property}>
			<option value="apartment">Квартира</option>
			<option value="house">Будинок</option>
			<option value="both">Квартира або будинок</option>
		</select>
	</label>
	<label class="check">
		<input name="enabled" type="checkbox" bind:checked={enabled} />
		Стежить
	</label>
	<button type="submit" disabled={saving}>{watch ? 'Зберегти' : 'Почати стежити'}</button>
	{#if notice}
		<p class="note" role="status">{notice}</p>
	{/if}
	<p class="note">
		Порожнє поле означає, що цієї межі немає. Місто може бути шляхом джерела: для LUN
		<code>/rent/kyiv/flats</code>, для DOM.RIA <code>/uk/arenda-kvartir/kiev/</code>.
	</p>
</form>

{#if watch}
	<section class="panel">
		<h2>Джерела</h2>
		{#if sources.length}
			{#each sources as source (source.id)}
				<p class="watching">
					<span class="dot" class:live={source.status === 'healthy'} class:bad={source.status === 'error'}
					></span>
					{sourceName(source.type)} · {sourceStatus(source.status, source.lastError)}
				</p>
				{#if source.lastError}
					<p class="note">{source.lastError}</p>
				{/if}
			{/each}
		{:else}
			<p class="note">Джерело не підключене.</p>
		{/if}
	</section>

	<section class="panel">
		<h2>Остання перевірка</h2>
		{#if run}
			<p class="note">Завершено {ago(run.finishedAt)} · {Math.max(1, Math.round(run.durationMs / 1000))} с</p>
			<ul class="facts">
				<li><span>Отримано</span><span>{run.received}</span></li>
				<li><span>Нових</span><span>{run.new}</span></li>
				<li><span>Змінених</span><span>{run.changed}</span></li>
				<li><span>Підійшли</span><span>{run.matched}</span></li>
				<li><span>Відсіяно</span><span>{run.rejected}</span></li>
				<li><span>Уже бачені</span><span>{run.duplicates}</span></li>
				{#if run.failed > 0}
					<li><span>Не вдалося прочитати</span><span>{run.failed}</span></li>
				{/if}
			</ul>
			{#each run.errors as message, index (index)}
				<p class="note">{message}</p>
			{/each}
			<p class="note">{nextCheck}</p>
		{:else}
			<p class="note">Перевірок ще не було.</p>
		{/if}
	</section>
{/if}

<section class="panel">
	<h2>Ключ Google</h2>
	<p class="note">
		Потрібен лише для пошуку біля адреси: Google перетворює назву місця на його ідентифікатор. Програма
		просить тільки ідентифікатор, а такі запити в Google безкоштовні.
	</p>
	{#if keyState.set}
		<p class="note">
			{keyState.fromEnv ? 'Ключ береться зі змінної GAPI_KEY' : 'Ключ збережено'}, закінчується на
			<code>{keyState.hint}</code>.
		</p>
	{/if}
	<form
		class="editor"
		onsubmit={(event) => {
			event.preventDefault();
			storeKey(keyInput.trim());
		}}
	>
		<label>
			{keyState.set ? 'Новий ключ' : 'Ключ'}
			<input
				name="googleKey"
				type="password"
				bind:value={keyInput}
				autocomplete="off"
				spellcheck="false"
				placeholder="AIza…"
			/>
		</label>
		<div class="actions">
			<button type="submit" disabled={keySaving || !keyInput.trim()}>Перевірити і зберегти</button>
			{#if keyState.set && !keyState.fromEnv}
				<button type="button" class="secondary" disabled={keySaving} onclick={() => storeKey('')}>
					Прибрати ключ
				</button>
			{/if}
		</div>
		{#if keyNotice}
			<p class="note" role="status">{keyNotice}</p>
		{/if}
	</form>
</section>

