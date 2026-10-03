import fs from 'node:fs/promises';

import { exec as syncExec } from 'child_process';
import { promisify } from 'util';
const execAsync = promisify(syncExec);
import minimist from 'minimist';
import path from 'path';
import { build } from 'vite';

import { BASE_PATH, getBaseConfig } from './vite.config.mjs';

const WORKER_BASE_PATH = path.resolve(BASE_PATH, 'worker');

const workers = {
	local_worker: path.resolve(WORKER_BASE_PATH, 'local_worker.ts'),
	net_worker: path.resolve(WORKER_BASE_PATH, 'net_worker.ts'),
	sim_worker: path.resolve(WORKER_BASE_PATH, 'sim_worker.ts'),
};

const args = minimist(process.argv.slice(2), { boolean: ['watch'] });

const buildWorkers = async () => {
	const { stdout } = await execAsync('go env GOROOT');
	const GO_ROOT = stdout.trim();
	// Go 1.24 moved wasm_exec.js from misc/wasm to lib/wasm. CI pins 1.23.x, so only a contributor
	// on a newer toolchain hits this, and the failure reads as a missing file rather than a version.
	const wasmExecutableCandidates = [path.join(GO_ROOT, '/lib/wasm/wasm_exec.js'), path.join(GO_ROOT, '/misc/wasm/wasm_exec.js')];
	let wasmFile: string | undefined;
	for (const candidate of wasmExecutableCandidates) {
		try {
			wasmFile = await fs.readFile(candidate, 'utf8');
			break;
		} catch {
			// try the other layout
		}
	}
	if (wasmFile === undefined) {
		throw new Error(`wasm_exec.js not found under ${GO_ROOT}; looked in ${wasmExecutableCandidates.join(' and ')}`);
	}

	Object.entries(workers).forEach(async ([name, sourcePath]) => {
		const baseConfig = getBaseConfig({ command: 'build', mode: 'production' });
		await build({
			...baseConfig,
			configFile: false,
			plugins: [
				{
					name: 'add-wasm-exec-file',
					transform: async (code, id) => {
						if (id.includes('sim_worker.ts')) {
							code = wasmFile + code;
						}
						return code;
					},
				},
			],
			build: {
				...baseConfig.build,
				watch: args.watch === true ? {} : undefined,
				minify: false,
				emptyOutDir: false,
				lib: {
					entry: { [name]: sourcePath },
					name: `${name}.js`,
					formats: ['cjs'],
				},
			},
		});
	});
};

buildWorkers();
