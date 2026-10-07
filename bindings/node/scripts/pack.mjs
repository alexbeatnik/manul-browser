#!/usr/bin/env node
/**
 * Build the npm tarballs: one per engine binary, and the main package.
 *
 *     node scripts/pack.mjs --binaries <dir> --out <dir>
 *
 * `--binaries` holds one directory per Go target, named `<goos>_<goarch>` and
 * containing `manul` or `manul.exe` — what `bindings/build-packages.sh` leaves
 * in `dist/bin`. Nothing is published from here; the tarballs are what
 * `npm publish <file>` is later pointed at.
 *
 * The main package's committed package.json carries no optionalDependencies on
 * purpose. They are added here, at the version being packed, so the dependency
 * list cannot name a different version from the package that carries it — and
 * so `npm ci` in a checkout never goes looking for platform packages that a
 * given commit has not published yet.
 */
import { execFileSync } from 'node:child_process';
import {
  chmodSync,
  copyFileSync,
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const main = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'));

/**
 * Go target -> the names Node gives the same machine. `platformPackage()` in
 * src/binary.ts builds the package name from `process.platform` and
 * `process.arch`, so these have to be those spellings exactly.
 */
const TARGETS = {
  'linux_amd64': { os: 'linux', cpu: 'x64' },
  'linux_arm64': { os: 'linux', cpu: 'arm64' },
  'darwin_amd64': { os: 'darwin', cpu: 'x64' },
  'darwin_arm64': { os: 'darwin', cpu: 'arm64' },
  'windows_amd64': { os: 'win32', cpu: 'x64' },
  'windows_arm64': { os: 'win32', cpu: 'arm64' },
};

const packageName = ({ os, cpu }) => `@manul-browser/engine-${os}-${cpu}`;

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    const key = argv[i];
    if (!key?.startsWith('--') || argv[i + 1] === undefined) {
      throw new Error(`usage: pack.mjs --binaries <dir> --out <dir>`);
    }
    args[key.slice(2)] = argv[i + 1];
  }
  if (!args.binaries || !args.out) throw new Error(`usage: pack.mjs --binaries <dir> --out <dir>`);
  return { binaries: resolve(args.binaries), out: resolve(args.out) };
}

/** `npm pack` in dir, and move what it produced into out. */
function pack(dir, out) {
  // Through a shell because npm is a .cmd on Windows. Nothing here is
  // interpolated into the command line: the directory is passed as cwd.
  const stdout = execFileSync('npm pack --json', { cwd: dir, shell: true, encoding: 'utf8' });
  const [{ filename }] = JSON.parse(stdout);
  // npm names a scoped tarball `scope-name-1.2.3.tgz` but reports it with the
  // scope's slash on some versions.
  const produced = filename.replace(/^@/, '').replace('/', '-');
  renameSync(join(dir, produced), join(out, produced));
  return produced;
}

function platformPackage(target, binary, stage) {
  const { os, cpu } = TARGETS[target];
  const name = packageName({ os, cpu });
  const exe = os === 'win32' ? 'manul.exe' : 'manul';

  mkdirSync(join(stage, 'bin'), { recursive: true });
  copyFileSync(binary, join(stage, 'bin', exe));
  if (os !== 'win32') chmodSync(join(stage, 'bin', exe), 0o755);
  copyFileSync(join(root, 'LICENSE'), join(stage, 'LICENSE'));

  writeFileSync(
    join(stage, 'package.json'),
    JSON.stringify(
      {
        name,
        version: main.version,
        description: `The Manul engine binary for ${os}-${cpu}. Installed by manul-browser; not meant to be depended on directly.`,
        license: main.license,
        author: main.author,
        homepage: main.homepage,
        repository: main.repository,
        bugs: main.bugs,
        // npm installs an optional dependency only where these match, which is
        // what makes six of them in one dependency list cost one download.
        os: [os],
        cpu: [cpu],
        files: ['bin'],
      },
      null,
      2,
    ) + '\n',
  );
  writeFileSync(
    join(stage, 'README.md'),
    `# ${name}\n\n` +
      `The [Manul](${main.homepage}) engine binary for ${os}-${cpu}.\n\n` +
      `This package is an implementation detail of ` +
      `[\`manul-browser\`](https://www.npmjs.com/package/manul-browser), which lists it as an ` +
      `optional dependency so that npm installs exactly the binary that matches the machine. ` +
      `Install \`manul-browser\` instead.\n`,
  );
  return name;
}

function mainPackage(stage, optionalDependencies) {
  if (!existsSync(join(root, 'dist', 'index.js'))) {
    throw new Error('dist/ is missing — run `npm run build` before packing');
  }
  cpSync(join(root, 'dist'), join(stage, 'dist'), { recursive: true });
  copyFileSync(join(root, 'README.md'), join(stage, 'README.md'));
  copyFileSync(join(root, 'LICENSE'), join(stage, 'LICENSE'));

  const { scripts: _scripts, devDependencies: _dev, ...published } = main;
  writeFileSync(
    join(stage, 'package.json'),
    JSON.stringify({ ...published, optionalDependencies }, null, 2) + '\n',
  );
}

function run() {
  const { binaries, out } = parseArgs(process.argv.slice(2));
  mkdirSync(out, { recursive: true });
  const work = mkdtempSync(join(tmpdir(), 'manul-npm-'));

  try {
    const built = [];
    for (const target of Object.keys(TARGETS)) {
      const exe = target.startsWith('windows_') ? 'manul.exe' : 'manul';
      const binary = join(binaries, target, exe);
      if (!existsSync(binary) || !statSync(binary).isFile()) continue;

      // A tarball records the mode bits it finds on disk, and Windows has no
      // executable bit to find. A Linux or macOS engine packed here would install
      // as a file nobody can run — the same reason hatch_build.py refuses to
      // build those wheels on Windows.
      if (process.platform === 'win32' && !target.startsWith('windows_')) {
        throw new Error(
          `${target}: cannot pack a non-Windows engine on Windows — the tarball would lose ` +
            `its executable bit. Build these on a POSIX host (the publish workflow does).`,
        );
      }

      const stage = join(work, target);
      mkdirSync(stage);
      const name = platformPackage(target, binary, stage);
      built.push({ name, file: pack(stage, out) });
    }
    if (built.length === 0) throw new Error(`no engine binaries found under ${binaries}`);

    // The main package always names all six, whichever were packed here: its
    // dependency list describes the release, not this machine.
    const optionalDependencies = Object.fromEntries(
      Object.values(TARGETS).map((t) => [packageName(t), main.version]),
    );
    const stage = join(work, 'main');
    mkdirSync(stage);
    mainPackage(stage, optionalDependencies);
    const mainFile = pack(stage, out);

    for (const { name, file } of built) console.log(`  ${file}  (${name})`);
    console.log(`  ${mainFile}  (${main.name})`);
    if (built.length < Object.keys(TARGETS).length) {
      console.log(
        `\n  ${built.length} of ${Object.keys(TARGETS).length} platform packages built. ` +
          `${main.name} depends on all of them — do not publish it from this set.`,
      );
    }
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}

try {
  run();
} catch (err) {
  // A packaging mistake is a message for a person, not a stack trace.
  console.error(`pack: ${err.message}`);
  process.exit(1);
}
