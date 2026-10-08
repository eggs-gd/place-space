<script>
	import '../app.css';

	let { children } = $props();
</script>

<svelte:head>
	<title>Place Space</title>
	<meta
		name="description"
		content="Локальний монітор оренди. Перевіряє LUN і DOM.RIA та показує оголошення у ваших межах."
	/>
</svelte:head>

{@render children()}
