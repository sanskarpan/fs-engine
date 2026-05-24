import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

const moduleRoot = resolve(process.cwd(), 'node_modules/flatted/golang');
const goModPath = resolve(moduleRoot, 'go.mod');

try {
  mkdirSync(dirname(goModPath), { recursive: true });
  writeFileSync(
    goModPath,
    'module github.com/web-deps/flatted-golang\n\ngo 1.20\n',
    'utf8',
  );
} catch (error) {
  console.warn('patch-flatted-go-module: skipped', error);
}
