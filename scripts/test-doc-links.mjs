import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { checkRepository, parseMarkdown, headingIds } from './check-doc-links.mjs';

const script = fileURLToPath(new URL('./check-doc-links.mjs', import.meta.url));
function repository(t, entries = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'r15-doc-links-'));
  t.after(() => {
    assert.equal(path.dirname(root), path.resolve(os.tmpdir()));
    assert(path.basename(root).startsWith('r15-doc-links-'));
    fs.rmSync(root, { recursive: true, force: true });
  });
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  const write = (file, content) => { fs.mkdirSync(path.dirname(path.join(root, file)), { recursive: true }); fs.writeFileSync(path.join(root, file), content); };
  git('init', '--quiet');
  for (const [file, content] of Object.entries({ 'README.md': '# Root\n', ...entries })) write(file, content);
  git('add', '--all');
  const run = (...args) => spawnSync(process.execPath, [script, ...args], { cwd: root, encoding: 'utf8' });
  return { root, git, write, run };
}

test('inline links, images, titles, angle targets, escaped and balanced parentheses', () => {
  const parsed = parseMarkdown('[a](one.md "title") ![b](<two words.png>) [c](a(b).md) [d](a\\(b\\).md)');
  assert.deepEqual(parsed.links.map(x => x.target), ['one.md', 'two words.png', 'a(b).md', 'a(b).md']);
  assert.deepEqual(parsed.issues, []);
});

test('nested badge links retain image and outer destination', () => {
  assert.deepEqual(parseMarkdown('[![badge](image.png)](target.md)').links.map(x => x.target), ['target.md', 'image.png']);
});

test('reference forms, case and whitespace labels, first definition wins', () => {
  const text = '[One][KEY] [key][] [key]\n\n[key]: <target.md> "title"\n[KEY]: wrong.md\n';
  assert.deepEqual(parseMarkdown(text).links.map(x => x.target), ['target.md', 'target.md', 'target.md']);
  assert.equal(parseMarkdown('[指令][参数]').links.length, 0);
});

test('code fences of both types, inline code, comments and escaped brackets are ignored', () => {
  const text = '````md\n[x](bad)\n```\n[x](bad2)\n````\n~~~\n[x](bad3)\n~~~\n`[x](bad4)` <!-- [x](bad5) --> \\[x](bad6)\n[x](ok.md)';
  assert.deepEqual(parseMarkdown(text).links.map(x => [x.target, x.line]), [['ok.md', 10]]);
});

test('HTML image and anchor destinations are checked', () => {
  assert.deepEqual(parseMarkdown('<img src="img.png"><a href=page.md>x</a>').links.map(x => x.target), ['img.png', 'page.md']);
});

test('unsupported link syntax is a located issue, not silently successful', () => {
  const parsed = parseMarkdown('intro\n[x](bad\n\n[key]:\n  target.md');
  assert.equal(parsed.issues.length, 2);
  assert.deepEqual(parsed.issues.map(x => x.line).sort(), [2, 4]);
});

test('GitHub heading rules include Chinese, formatting, code, link text and Setext', () => {
  const ids = headingIds('# 中文 **标题** _斜体_ `code_name` [链接](x.md) ##\n\nSetext\n======\n\n# C#\n# A  B\n');
  for (const expected of ['中文-标题-斜体-code_name-链接', 'setext', 'c', 'a--b']) assert(ids.has(expected), expected);
});

test('duplicate suffix collisions and custom anchors use independent namespaces', () => {
  const ids = headingIds('<a name="same"></a>\n# Same\n# Same\n# Same-1\n# Same\n<div id="custom"></div>\n```\n# Hidden\n<a id="hidden"></a>\n```');
  assert.deepEqual([...ids].sort(), ['custom', 'same', 'same-1', 'same-1-1', 'same-2']);
});

test('tracked files, encoded spaces, Chinese, root paths, directories and same-file fragments', t => {
  const r = repository(t, { 'docs/local-admin/a.md': '# 标题\n[x](#标题) [x](/README.md#root) [x](../../assets/%E4%B8%AD%20%E6%96%87.png) [x](../../assets/)', 'assets/中 文.png': 'image' });
  const report = checkRepository(r.root);
  assert.equal(report.documents, 2);
  assert.equal(report.localLinks, 4);
  assert.equal(report.anchors, 2);
  assert.deepEqual(report.errors, []);
  assert.equal(r.run().status, 0);
});

test('new staged docs included, untracked and out-of-scope docs excluded', t => {
  const r = repository(t);
  r.write('docs/local-admin/new.md', '[x](missing.md)');
  r.write('docs/local-notes/private.md', '[x](missing.md)');
  assert.equal(checkRepository(r.root).documents, 1);
  r.git('add', 'docs/local-admin/new.md');
  assert.equal(checkRepository(r.root).documents, 2);
  assert.equal(r.run().status, 1);
});

test('missing, wrong-case, outside, private and absolute paths fail with locations', t => {
  const links = ['missing.md', 'readme.md', '../outside.md', '.local/private.md', 'C:/secret.md', 'file:///secret', '%ZZ', 'README.md#missing'];
  const r = repository(t, { 'README.md': links.map(x => `[x](${x})`).join('\n'), '.local/private.md': 'private' });
  const report = checkRepository(r.root);
  assert.equal(report.errors.length, links.length);
  assert.deepEqual(report.errors.map(x => x.line), links.map((_, i) => i + 1));
  assert.equal(r.run().status, 1);
  assert.match(r.run().stderr, /README.md:8:/);
});

test('external links never fetched and non-Markdown fragments explicitly unverified', t => {
  const r = repository(t, { 'README.md': '[x](https://invalid.example/a) [x](//invalid.example/a) [x](mailto:x@example.test) [x](source.go#L1)', 'source.go': 'package main' });
  const report = checkRepository(r.root);
  assert.equal(report.external, 3);
  assert.equal(report.anchors, 0);
  assert.equal(report.unverifiedFragments.length, 1);
  assert.deepEqual(report.errors, []);
  assert.match(r.run().stdout, /未验证片段 source.go#L1/);
});

test('deleted tracked sources and malformed UTF-8 fail without hiding other errors', t => {
  const r = repository(t, { 'docs/local-admin/deleted.md': '# Gone', 'docs/local-admin/encoding.md': Buffer.from([0xff]), 'README.md': '[x](missing.md)' });
  fs.unlinkSync(path.join(r.root, 'docs/local-admin/deleted.md'));
  const report = checkRepository(r.root);
  assert.equal(report.errors.length, 3);
  assert.equal(r.run().status, 1);
});

test('CLI misuse exits 2 and missing Git repository exits 1', t => {
  const r = repository(t);
  assert.equal(r.run('--fix').status, 2);
  const result = spawnSync(process.execPath, [script], { cwd: os.tmpdir(), encoding: 'utf8' });
  assert.equal(result.status, 1);
});

test('repeated runs have stable output and preserve file bytes and Git status', t => {
  const r = repository(t, { 'README.md': '[x](missing.md)\r\n', 'docs/local-admin/b.md': '[x](#absent)' });
  const status = r.git('status', '--porcelain=v1'), bytes = fs.readFileSync(path.join(r.root, 'README.md'));
  const first = r.run(), second = r.run();
  assert.equal(first.stdout, second.stdout);
  assert.equal(first.stderr, second.stderr);
  assert.deepEqual(fs.readFileSync(path.join(r.root, 'README.md')), bytes);
  assert.equal(r.git('status', '--porcelain=v1'), status);
});

test('inline code preserves literal underscores in headings and exact backtick delimiters', () => {
  assert(headingIds('# `a_b_c` and a_b_c').has('a_b_c-and-a_b_c'));
  assert.deepEqual(parseMarkdown('`` `[x](bad)` `` [x](ok.md)').links.map(x => x.target), ['ok.md']);
});
