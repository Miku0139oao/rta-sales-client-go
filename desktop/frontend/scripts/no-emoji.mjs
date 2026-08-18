import { readdir, readFile } from 'node:fs/promises';
import { extname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = fileURLToPath(new URL('../', import.meta.url));
const sourceExtensions = new Set(['.css', '.html', '.js', '.json', '.mjs', '.svelte', '.ts']);
const ignoredDirectories = new Set(['node_modules', 'dist', 'dist-web', 'coverage', '.git']);
const emojiPattern = /\p{Extended_Pictographic}|\p{Emoji_Presentation}/gu;
const violations = [];

async function scan(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isDirectory() && ignoredDirectories.has(entry.name)) continue;
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      await scan(path);
      continue;
    }
    if (!sourceExtensions.has(extname(entry.name))) continue;

    const text = await readFile(path, 'utf8');
    const lines = text.split(/\r?\n/);
    for (const [index, line] of lines.entries()) {
      emojiPattern.lastIndex = 0;
      const matches = [...line.matchAll(emojiPattern)];
      if (matches.length > 0) {
        violations.push(`${relative(frontendRoot, path)}:${index + 1} ${matches.map((match) => match[0]).join(' ')}`);
      }
    }
  }
}

await scan(frontendRoot);

if (violations.length > 0) {
  console.error('Emoji characters are not allowed in the desktop frontend:');
  violations.forEach((violation) => console.error(`  ${violation}`));
  process.exitCode = 1;
} else {
  console.log('No emoji characters found.');
}
