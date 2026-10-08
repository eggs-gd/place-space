<script lang="ts">
	import { page } from '$app/state';
</script>

<section class="empty">
	<h1>{page.status}</h1>
	<p>{page.error?.message ?? 'Щось пішло не так'}</p>
	<a href="/">До місць</a>
</section>
