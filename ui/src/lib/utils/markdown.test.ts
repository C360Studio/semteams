import { describe, test, expect } from "vitest";
import { renderMarkdown, escapeHtml } from "./markdown";

describe("escapeHtml", () => {
  test("escapes the five HTML metacharacters", () => {
    expect(escapeHtml(`& < > " '`)).toBe("&amp; &lt; &gt; &quot; &#39;");
  });

  test("leaves ordinary text untouched", () => {
    expect(escapeHtml("hello world 123")).toBe("hello world 123");
  });
});

describe("renderMarkdown — empty / whitespace", () => {
  test("empty string → empty", () => {
    expect(renderMarkdown("")).toBe("");
  });

  test("whitespace-only → empty", () => {
    expect(renderMarkdown("   \n  \n")).toBe("");
  });
});

describe("renderMarkdown — headings", () => {
  test("# → h3, ## → h4, ### → h5", () => {
    expect(renderMarkdown("# Title")).toContain("<h3 class=\"md-h\">Title</h3>");
    expect(renderMarkdown("## Sub")).toContain("<h4 class=\"md-h\">Sub</h4>");
    expect(renderMarkdown("### Deep")).toContain("<h5 class=\"md-h\">Deep</h5>");
  });

  test("more than 3 hashes is not a heading (degrades to paragraph)", () => {
    const out = renderMarkdown("#### Nope");
    expect(out).not.toContain("<h");
    expect(out).toContain("<p");
    expect(out).toContain("#### Nope");
  });
});

describe("renderMarkdown — emphasis", () => {
  test("bold", () => {
    expect(renderMarkdown("a **bold** b")).toContain("<strong>bold</strong>");
  });

  test("italic with asterisk", () => {
    expect(renderMarkdown("a *em* b")).toContain("<em>em</em>");
  });

  test("italic with underscore", () => {
    expect(renderMarkdown("a _em_ b")).toContain("<em>em</em>");
  });

  test("snake_case is not italicised", () => {
    const out = renderMarkdown("the loop_id field");
    expect(out).not.toContain("<em>");
    expect(out).toContain("loop_id");
  });

  test("bold wins over italic for ** runs", () => {
    const out = renderMarkdown("**strong**");
    expect(out).toContain("<strong>strong</strong>");
    expect(out).not.toContain("<em>");
  });
});

describe("renderMarkdown — inline code", () => {
  test("backticks become <code>", () => {
    expect(renderMarkdown("run `go test`")).toContain("<code>go test</code>");
  });

  test("emphasis markers inside code are left literal", () => {
    const out = renderMarkdown("`a*b*c`");
    expect(out).toContain("<code>a*b*c</code>");
    expect(out).not.toContain("<em>");
  });
});

describe("renderMarkdown — links", () => {
  test("https link becomes an anchor with safe rel", () => {
    const out = renderMarkdown("[docs](https://example.com/x)");
    expect(out).toContain('href="https://example.com/x"');
    expect(out).toContain('rel="noopener noreferrer nofollow"');
    expect(out).toContain(">docs</a>");
  });

  test("http and mailto are allowed", () => {
    expect(renderMarkdown("[a](http://x.io)")).toContain('href="http://x.io"');
    expect(renderMarkdown("[b](mailto:x@y.io)")).toContain(
      'href="mailto:x@y.io"',
    );
  });

  test("relative / schemeless links degrade to plain text", () => {
    const out = renderMarkdown("[home](/dashboard)");
    expect(out).not.toContain("<a");
    expect(out).toContain("home");
  });
});

describe("renderMarkdown — lists", () => {
  test("unordered list", () => {
    const out = renderMarkdown("- one\n- two");
    expect(out).toContain('<ul class="md-list">');
    expect(out).toContain("<li>one</li>");
    expect(out).toContain("<li>two</li>");
  });

  test("ordered list", () => {
    const out = renderMarkdown("1. first\n2. second");
    expect(out).toContain('<ol class="md-list">');
    expect(out).toContain("<li>first</li>");
    expect(out).toContain("<li>second</li>");
  });

  test("emphasis renders inside list items", () => {
    const out = renderMarkdown("- a **b** c");
    expect(out).toContain("<li>a <strong>b</strong> c</li>");
  });
});

describe("renderMarkdown — fenced code", () => {
  test("fence becomes pre/code with verbatim body", () => {
    const out = renderMarkdown("```\nline1\nline2\n```");
    expect(out).toContain('<pre class="md-code"><code>line1\nline2</code></pre>');
  });

  test("markdown inside a fence is not interpreted", () => {
    const out = renderMarkdown("```\n# not a heading\n**not bold**\n```");
    expect(out).not.toContain("<h3");
    expect(out).not.toContain("<strong>");
    expect(out).toContain("# not a heading");
  });

  test("language hint on the fence is tolerated", () => {
    const out = renderMarkdown("```go\nfmt.Println()\n```");
    expect(out).toContain("<pre");
    expect(out).toContain("fmt.Println()");
  });
});

describe("renderMarkdown — blockquote", () => {
  test("consecutive > lines fold into one blockquote", () => {
    const out = renderMarkdown("> first\n> second");
    expect(out).toContain('<blockquote class="md-quote">first second</blockquote>');
  });
});

describe("renderMarkdown — paragraphs", () => {
  test("intra-paragraph newlines become <br>", () => {
    const out = renderMarkdown("line one\nline two");
    expect(out).toContain("line one<br>line two");
  });

  test("blank line separates paragraphs", () => {
    const out = renderMarkdown("para one\n\npara two");
    const paras = out.match(/<p class="md-p">/g) ?? [];
    expect(paras.length).toBe(2);
  });
});

describe("renderMarkdown — a realistic autoresearch rollup", () => {
  test("renders a mixed document without dropping content", () => {
    const doc = [
      "## Result",
      "",
      "Optimized `go test ./...` from **1.20s to 0.85s** (29% faster).",
      "",
      "Kept iterations:",
      "- parallelised independent suites",
      "- dropped a redundant fixture rebuild",
    ].join("\n");
    const out = renderMarkdown(doc);
    expect(out).toContain("<h4 class=\"md-h\">Result</h4>");
    expect(out).toContain("<code>go test ./...</code>");
    expect(out).toContain("<strong>1.20s to 0.85s</strong>");
    expect(out).toContain("<li>parallelised independent suites</li>");
  });
});
