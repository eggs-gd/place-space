export type Reason = {
	field: string;
	passed: boolean;
	message: string;
};

export type Card = {
	id: string;
	source: string;
	url: string;
	title: string;
	priceAmount: number | null;
	currency: string;
	location: string;
	address: string;
	rooms: number | null;
	area: number | null;
	floor: number | null;
	totalFloors: number | null;
	image: string;
	publishedAt: string | null;
	firstSeenAt: string;
	lastSeenAt: string;
	isNew: boolean;
	status: string;
	reasons: Reason[];
};

export type HistoryPoint = {
	seenAt: string;
	priceAmount: number;
	currency: string;
};

export type Duplicate = {
	id: string;
	source: string;
	url: string;
	title: string;
	priceAmount: number | null;
	currency: string;
};

export type ListingDetail = Card & {
	description: string;
	images: string[];
	history: HistoryPoint[];
	duplicates: Duplicate[];
};

export type Watch = {
	id: string;
	name: string;
	enabled: boolean;
	city: string;
	deal: string;
	properties: string[];
	priceMin: number | null;
	priceMax: number | null;
	currency: string;
	roomsMin: number | null;
	areaMin: number | null;
	pollIntervalSeconds: number;
};

export type Source = {
	id: string;
	type: string;
	enabled: boolean;
	status: string;
	lastSuccessAt: string | null;
	lastErrorAt: string | null;
	lastError: string;
};

export type Run = {
	sourceType: string;
	startedAt: string;
	finishedAt: string;
	durationMs: number;
	received: number;
	new: number;
	changed: number;
	matched: number;
	rejected: number;
	duplicates: number;
	failed: number;
	errors: string[];
};

export type Overview = {
	watch: Watch | null;
	summary: { places: number; new: number; rejected: number };
	source: Source | null;
	sources: Source[];
	run: Run | null;
};

export type Feed = {
	status: string;
	items: Card[];
};
