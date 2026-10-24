import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const blank = value => value.replace(/[^\n]/g, ' ');
const unescape = value => value.replace(/\\([!"#$%&'()*+,\-./:;<=>?@[\]\\^_`{|}~])/g, '$1');
const labelKey = value => value.trim().replace(/\s+/g, ' ').toLowerCase();

function withoutBlocks(text) {
  let fence = null;
  return text.replace(/\r\n/g, '\n').replace(/^\uFEFF/, '').split('\n').map(line => {
    const match = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
    if (fence) {
      if (match && match[1][0] === fence[0] && match[1].length >= fence.length && !match[2].trim()) fence = null;
      return blank(line);
    }
    if (match) { fence = match[1]; return blank(line); }
    return line;
  }).join('\n').replace(/<!--[\s\S]*?-->/g, blank);
}

function withoutCode(text) {
  return text.replace(/(?<!`)(`+)(?!`)([\s\S]*?)(?<!`)\1(?!`)/g, blank);
}

// The supported subset is documented in document-check.md; this is not a Markdown renderer.
function destination(text, start, inline) {
  let i = start;
  while (/\s/.test(text[i] ?? '') && i < text.length) i++;
  if (!inline && i === text.length) return null;
  let value = '';
  if (text[i] === '<') {
    const end = text.indexOf('>', ++i);
    if (end < 0 || text.slice(i, end).includes('\n')) return null;
    value = text.slice(i, end); i = end + 1;
  } else {
    let depth = 0;
    for (; i < text.length; i++) {
      const c = text[i];
      if (c === '\\' && i + 1 < text.length) { value += c + text[++i]; continue; }
      if (/\s/.test(c) || (c === ')' && depth === 0)) break;
      if (c === '(') depth++;
      if (c === ')') depth--;
      value += c;
    }
    if (depth) return null;
  }
  while (i < text.length && /\s/.test(text[i])) i++;
  if (['"', "'", '('].includes(text[i])) {
    const closing = text[i] === '(' ? ')' : text[i];
    i++;
    while (i < text.length && text[i] !== closing) { if (text[i] === '\\') i++; i++; }
    if (i === text.length) return null;
    i++;
    while (i < text.length && /\s/.test(text[i])) i++;
  }
  if (inline ? text[i] !== ')' : i !== text.length) return null;
  return { target: unescape(value), end: inline ? i + 1 : i };
}

export function parseMarkdown(source) {
  const blocks = withoutBlocks(source);
  const definitions = new Map(), links = [], issues = [];
  let text = blocks.replace(/^ {0,3}\[([^\]\n]+)\]:[^\n]*$/gm, (line, label, offset) => {
    const value = destination(line, line.indexOf(']:') + 2, false);
    if (!value) issues.push({ offset, target: line.trim(), reason: '不支持的引用定义，请使用同一行目标与标题' });
    else if (!definitions.has(labelKey(label))) definitions.set(labelKey(label), value.target);
    return blank(line);
  });
  text = withoutCode(text);
  const stack = [];
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '\\') { i++; continue; }
    if (text[i] === '[') { stack.push(i); continue; }
    if (text[i] !== ']' || !stack.length) continue;
    const start = stack.pop(), label = text.slice(start + 1, i);
    if (text[i + 1] === '(') {
      const value = destination(text, i + 2, true);
      if (value) { links.push({ offset: start, target: value.target }); i = value.end - 1; }
      else issues.push({ offset: start, target: text.slice(start, text.indexOf('\n', start) < 0 ? text.length : text.indexOf('\n', start)), reason: '不支持或未闭合的行内链接' });
    } else if (text[i + 1] === '[') {
      const end = text.indexOf(']', i + 2);
      if (end < 0) continue;
      const key = labelKey(text.slice(i + 2, end) || label);
      // Undefined references are ordinary text in Markdown (also used by QQ command examples).
      if (definitions.has(key)) { links.push({ offset: start, target: definitions.get(key) }); i = end; }
    } else if (definitions.has(labelKey(label))) links.push({ offset: start, target: definitions.get(labelKey(label)) });
  }
  for (const m of text.matchAll(/<(?:a|img)\b[^>]*>/gi)) {
    for (const attr of m[0].matchAll(/\b(?:href|src)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/gi)) {
      links.push({ offset: m.index, target: attr[1] ?? attr[2] ?? attr[3] });
    }
  }
  const located = item => ({ ...item, line: text.slice(0, item.offset).split('\n').length });
  return { links: links.map(located).sort((a, b) => a.offset - b.offset), issues: issues.map(located) };
}

export function headingIds(source) {
  const text = withoutBlocks(source), ids = new Set(), generated = new Set();
  const add = heading => {
    let title = heading;
    // Use the same balanced destination parser as body links.
    for (const match of [...withoutCode(heading).matchAll(/!?\[([^\]]*)\]\(/g)].reverse()) {
      const value = destination(heading, match.index + match[0].length, true);
      if (value) title = title.slice(0, match.index) + heading.slice(match.index + (match[0].startsWith('!') ? 2 : 1), match.index + match[0].length - 2) + title.slice(value.end);
    }
    title = title.replace(/<[^>]*>/g, '');
    title = title.split(/((?<!`)(`+)(?!`).*?(?<!`)\2(?!`))/g).filter((_, i) => i % 3 !== 2).map(part => {
      if (part.startsWith('`') && part.endsWith('`')) return part.replace(/^`+|`+$/g, '');
      return part.replace(/(\*\*|~~|\*)(.*?)\1/g, '$2').replace(/(?<![\p{L}\p{N}])(__|_)(.*?)\1(?![\p{L}\p{N}])/gu, '$2');
    }).join('');
    const base = unescape(title).trim().toLowerCase().replace(/[^\p{L}\p{N}\p{M}_\- ]/gu, '').replace(/ /g, '-');
    let id = base, n = 0;
    while (generated.has(id)) id = base + '-' + ++n;
    generated.add(id); ids.add(id);
  };
  const lines = text.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const atx = lines[i].match(/^ {0,3}#{1,6}(?:[ \t]+(.*?)|$)$/);
    if (atx) add((atx[1] ?? '').replace(/[ \t]+#+[ \t]*$/, ''));
    else if (i + 1 < lines.length && lines[i].trim() && /^ {0,3}(?:=+|-+)\s*$/.test(lines[i + 1])) { add(lines[i]); i++; }
  }
  for (const tag of withoutCode(text).matchAll(/<[a-z][^>]*>/gi)) {
    for (const m of tag[0].matchAll(/\b(?:id|name)\s*=\s*(?:"([^"]*)"|'([^']*)')/g)) ids.add(m[1] ?? m[2]);
  }
  return ids;
}

export function checkRepository(cwd) {
  const root = fs.realpathSync(execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8' }).trim());
  const tracked = execFileSync('git', ['ls-files', '-z'], { cwd: root, encoding: 'utf8' }).split('\0').filter(Boolean);
  const files = new Set(tracked), directories = new Set(['.']);
  for (const file of tracked) {
    let dir = path.posix.dirname(file);
    while (dir !== '.') { directories.add(dir); dir = path.posix.dirname(dir); }
  }
  const sources = tracked.filter(file => /\.md$/i.test(file) && (file.startsWith('docs/local-admin/') || ['README.md', 'AGENTS.md'].includes(file))).sort();
  const result = { documents: sources.length, localLinks: 0, anchors: 0, external: 0, unverifiedFragments: [], errors: [] };
  const cache = new Map();
  const read = file => {
    if (!cache.has(file)) {
      const absolute = path.join(root, file), actual = fs.realpathSync(absolute);
      const relative = path.relative(root, actual);
      if (relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) throw new Error('目标指向仓库外');
      cache.set(file, new TextDecoder('utf-8', { fatal: true }).decode(fs.readFileSync(absolute)));
    }
    return cache.get(file);
  };
  const headings = new Map();
  for (const file of sources) {
    let parsed;
    try { parsed = parseMarkdown(read(file)); }
    catch (error) { result.errors.push({ file, line: 1, target: file, reason: '读取失败: ' + error.message }); continue; }
    result.errors.push(...parsed.issues.map(issue => ({ file, ...issue })));
    for (const link of parsed.links) {
      const { target, line } = link;
      const fail = reason => result.errors.push({ file, line, target, reason });
      if (/^(?:[a-z]:|file:|\\)/i.test(target)) { fail('本机绝对路径不可交付'); continue; }
      if (/^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(target)) { result.external++; continue; }
      result.localLinks++;
      try {
        const hash = target.indexOf('#');
        const name = decodeURIComponent(hash < 0 ? target : target.slice(0, hash));
        const fragment = hash < 0 ? '' : decodeURIComponent(target.slice(hash + 1));
        if (name.includes('?') || name.includes('\\')) { fail('本地查询参数或反斜线路径不受支持'); continue; }
        const dest = name ? path.posix.normalize(name.startsWith('/') ? name.slice(1) : path.posix.join(path.posix.dirname(file), name)) : file;
        if (dest === '..' || dest.startsWith('../') || /(?:^|\/)(?:\.local|local-notes)(?:\/|$)/.test(dest)) { fail('目标位于仓库外或私有目录'); continue; }
        const normalized = dest.replace(/\/$/, '') || '.';
        if (!files.has(normalized) && !directories.has(normalized)) { fail('目标未跟踪或大小写不一致'); continue; }
        if (!fs.existsSync(path.join(root, normalized))) { fail('目标在工作区缺失'); continue; }
        const resolved = path.relative(root, fs.realpathSync(path.join(root, normalized)));
        if (resolved === '..' || resolved.startsWith('..' + path.sep) || path.isAbsolute(resolved)) { fail('目标指向仓库外'); continue; }
        if (fragment) {
          if (!/\.md$/i.test(normalized)) result.unverifiedFragments.push({ file, line, target });
          else {
            result.anchors++;
            if (!headings.has(normalized)) headings.set(normalized, headingIds(read(normalized)));
            if (!headings.get(normalized).has(fragment)) fail('Markdown锚点不存在');
          }
        }
      } catch (error) { fail('解析或读取失败: ' + error.message); }
    }
  }
  return result;
}

function main() {
  if (process.argv.length !== 2) { console.error('用法: node scripts/check-doc-links.mjs'); return 2; }
  try {
    const report = checkRepository(process.cwd());
    console.log(`范围: Git跟踪的 docs/local-admin/**/*.md、README.md、AGENTS.md（读取工作区内容，新增文件需暂存）`);
    console.log(`文档 ${report.documents}；本地引用 ${report.localLinks}；锚点 ${report.anchors}；外部引用未联网验证 ${report.external}；非Markdown片段未验证 ${report.unverifiedFragments.length}；错误 ${report.errors.length}`);
    console.log('语法范围: 行内/引用式链接、图片及HTML a/img；复杂Markdown布局、HTML实体和自动外链不保证解析，详见document-check.md。');
    for (const item of report.unverifiedFragments) console.log(`${item.file}:${item.line}: 未验证片段 ${item.target}`);
    for (const item of report.errors) console.error(`${item.file}:${item.line}: ${item.target} — ${item.reason}`);
    return report.errors.length ? 1 : 0;
  } catch (error) { console.error('文档检查失败: ' + error.message); return 1; }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) process.exitCode = main();
