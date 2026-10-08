import { cp, rm } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const web = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const from = resolve(web, 'build');
const to = resolve(web, '../internal/ui/dist');

await rm(to, { recursive: true, force: true });
await cp(from, to, { recursive: true });
