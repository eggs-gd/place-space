<script>
	const windows =
		'https://github.com/eggs-gd/place-space/releases/latest/download/place-space.exe';
	const mac =
		'https://github.com/eggs-gd/place-space/releases/latest/download/place-space-macos.zip';
</script>

<main class="shell">
	<p class="brand">Place Space</p>
	<h1>Оренда, яка з’являється сама</h1>
	<p class="lead">
		Локальна програма перевіряє LUN і DOM.RIA та показує оголошення у ваших межах. Нові — ті, що
		з’явилися за останню добу. База лишається на вашому комп’ютері.
	</p>
	<div class="actions">
		<a class="button" href={windows}>Завантажити для Windows</a>
		<a class="button" href={mac}>Завантажити для Mac</a>
	</div>
	<p class="note">На Mac перший запуск може вимагати правий клік і «Відкрити».</p>

	<figure>
		<img src="/screenshots/feed.png" alt="Стрічка оголошень у Place Space" />
		<figcaption>Стрічка місць, які підійшли під критерії.</figcaption>
	</figure>
	<figure>
		<img src="/screenshots/listing.png" alt="Картка одного оголошення" />
		<figcaption>Картка оголошення з фото і посиланням на джерело.</figcaption>
	</figure>

	<footer>
		<a href="https://github.com/eggs-gd/place-space">Код на GitHub</a>
	</footer>
</main>
