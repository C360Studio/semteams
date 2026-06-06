/**
 * renderMarkdown — minimal, dependency-free, XSS-safe markdown → HTML.
 *
 * Built for rendering LLM-authored text (model-call responses, artifact
 * prose) inside the agentic story view. The codebase carries no markdown
 * dependency by design (see ArtifactCard's header comment) and asserts
 * XSS-safety via `.attack.test.ts` suites; this renderer holds the same
 * contract so its output is safe to pass to `{@html}`:
 *
 *   1. EVERY HTML metacharacter in the source is escaped FIRST. Source
 *      text can therefore never introduce a tag, attribute, or entity —
 *      the only `<` characters in the output are ones this function emits.
 *   2. Only a fixed whitelist of tags is generated: h3–h5, p, br, strong,
 *      em, code, pre, ul, ol, li, a, blockquote.
 *   3. Links render an <a> only for http(s):// and mailto: schemes; any
 *      other scheme (javascript:, data:, …) drops to the link's plain
 *      escaped text — no script/URL-injection vector survives.
 *
 * Supported constructs are the subset LLMs actually emit: ATX headings
 * (#/##/### → h3/h4/h5), fenced code blocks (```), unordered lists (-/*),
 * ordered lists (1.), blockquotes (>), bold (**), italic (* or _), inline
 * code (`), and links ([text](url)). Tables, images, footnotes, setext
 * headings, and nested lists are intentionally out of scope — when an
 * unsupported construct appears it degrades to escaped plain text, never
 * to unsafe markup.
 */

const ESCAPE_MAP: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

/** Escape the five HTML metacharacters. Runs before any formatting. */
export function escapeHtml(src: string): string {
  return src.replace(/[&<>"']/g, (c) => ESCAPE_MAP[c]);
}

/**
 * Only http(s) and mailto links become anchors. `url` is already
 * HTML-escaped when this runs, so a `"` is `&quot;` and cannot break out
 * of the href attribute; the scheme check closes javascript:/data: etc.
 */
function isSafeUrl(escapedUrl: string): boolean {
  const lower = escapedUrl.trim().toLowerCase();
  return (
    lower.startsWith("http://") ||
    lower.startsWith("https://") ||
    lower.startsWith("mailto:")
  );
}

// Sentinel codepoint that brackets already-rendered inline spans (code,
// links) so later bold/italic passes can't reach inside them. A
// private-use codepoint — escaped source (which only ever contains the
// five escaped metacharacters as entities) won't collide with it.
const STASH = "\u0000";

/**
 * Apply inline formatting to a line of ALREADY-ESCAPED text. Code spans
 * and links are rendered first and stashed behind sentinels so emphasis
 * rules don't rewrite their innards, then restored at the end.
 */
function renderInline(escaped: string): string {
  const stash: string[] = [];
  const keep = (html: string): string => {
    stash.push(html);
    return `${STASH}${stash.length - 1}${STASH}`;
  };

  let s = escaped;

  // Inline code: `code` — content stays escaped, no emphasis inside.
  s = s.replace(/`([^`]+)`/g, (_m, code) => keep(`<code>${code}</code>`));

  // Links: [text](url). url has no whitespace or ')'; safe schemes only.
  s = s.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_m, text, url) =>
    isSafeUrl(url)
      ? keep(
          `<a href="${url}" target="_blank" rel="noopener noreferrer nofollow">${text}</a>`,
        )
      : text,
  );

  // Bold before italic so **x** isn't half-consumed by the * rule.
  s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  // Italic *x* — guard against a leftover ** boundary.
  s = s.replace(/(^|[^*])\*([^*\n]+)\*/g, "$1<em>$2</em>");
  // Italic _x_ — guard against snake_case (require non-word boundary).
  s = s.replace(/(^|[^_\w])_([^_\n]+)_/g, "$1<em>$2</em>");

  // Restore stashed spans.
  s = s.replace(
    new RegExp(`${STASH}(\\d+)${STASH}`, "g"),
    (_m, i) => stash[Number(i)],
  );
  return s;
}

const RE_FENCE = /^\s*```/;
const RE_HEADING = /^(#{1,3})\s+(.*)$/;
// Block detection runs on already-escaped lines, so the quote marker is the
// escaped entity `&gt;`, not a raw `>`. (Heading/list/fence markers — #, -,
// *, digits, backticks — aren't escaped, so they match their literal form.)
const RE_QUOTE = /^\s*&gt;\s?/;
const RE_ULIST = /^\s*[-*]\s+/;
const RE_OLIST = /^\s*\d+\.\s+/;
const RE_BLANK = /^\s*$/;

/** True when `line` opens a block construct (used to terminate paragraphs). */
function startsBlock(line: string): boolean {
  return (
    RE_FENCE.test(line) ||
    RE_HEADING.test(line) ||
    RE_QUOTE.test(line) ||
    RE_ULIST.test(line) ||
    RE_OLIST.test(line)
  );
}

/** Render markdown source to safe HTML. Empty/whitespace source → "". */
export function renderMarkdown(src: string): string {
  if (!src || src.trim() === "") return "";

  const lines = escapeHtml(src).split(/\r?\n/);
  const blocks: string[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block — body is verbatim (already escaped), no inline.
    if (RE_FENCE.test(line)) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !RE_FENCE.test(lines[i])) {
        body.push(lines[i]);
        i++;
      }
      i++; // consume the closing fence (or fall off the end)
      blocks.push(`<pre class="md-code"><code>${body.join("\n")}</code></pre>`);
      continue;
    }

    // ATX heading: # → h3, ## → h4, ### → h5 (kept small for a side panel).
    const h = line.match(RE_HEADING);
    if (h) {
      const level = h[1].length + 2;
      blocks.push(
        `<h${level} class="md-h">${renderInline(h[2].trim())}</h${level}>`,
      );
      i++;
      continue;
    }

    // Blockquote — fold consecutive `>` lines into one quote.
    if (RE_QUOTE.test(line)) {
      const quote: string[] = [];
      while (i < lines.length && RE_QUOTE.test(lines[i])) {
        quote.push(lines[i].replace(RE_QUOTE, ""));
        i++;
      }
      blocks.push(
        `<blockquote class="md-quote">${renderInline(quote.join(" "))}</blockquote>`,
      );
      continue;
    }

    // Unordered list.
    if (RE_ULIST.test(line)) {
      const items: string[] = [];
      while (i < lines.length && RE_ULIST.test(lines[i])) {
        items.push(`<li>${renderInline(lines[i].replace(RE_ULIST, ""))}</li>`);
        i++;
      }
      blocks.push(`<ul class="md-list">${items.join("")}</ul>`);
      continue;
    }

    // Ordered list.
    if (RE_OLIST.test(line)) {
      const items: string[] = [];
      while (i < lines.length && RE_OLIST.test(lines[i])) {
        items.push(`<li>${renderInline(lines[i].replace(RE_OLIST, ""))}</li>`);
        i++;
      }
      blocks.push(`<ol class="md-list">${items.join("")}</ol>`);
      continue;
    }

    // Blank line — block separator.
    if (RE_BLANK.test(line)) {
      i++;
      continue;
    }

    // Paragraph — gather until a blank line or the next block opener.
    const para: string[] = [];
    while (
      i < lines.length &&
      !RE_BLANK.test(lines[i]) &&
      !startsBlock(lines[i])
    ) {
      para.push(lines[i]);
      i++;
    }
    blocks.push(`<p class="md-p">${para.map(renderInline).join("<br>")}</p>`);
  }

  return blocks.join("\n");
}
