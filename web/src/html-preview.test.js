import assert from "node:assert/strict";
import { test } from "node:test";
import { annotationText, htmlPreviewURL } from "./html-preview.js";

test("relative asset resolution keeps routing scope and URL-encodes filenames", () => {
  const url = new URL(htmlPreviewURL({ projectID: 7, path: "docs/mock up/overview.html" }, { remote: "office", profile: "work", token: "t0k" }), "https://app.example");
  assert.equal(new URL("images/photo%20%231.png", url).pathname, "/api/html/office/work/projects/7/preview/t0k/-/docs/mock%20up/images/photo%20%231.png");
  assert.equal(new URL("../styles/main.css", url).pathname, "/api/html/office/work/projects/7/preview/t0k/-/docs/styles/main.css");
});

test("absolute documents use their own directory, including Unicode paths", () => {
  const url = htmlPreviewURL({ projectID: 2, path: "/tmp/maquetas/日本語/overview.html" });
  const parts = url.split("/");
  assert.equal(Buffer.from(parts.at(-2), "base64url").toString(), "/tmp/maquetas/日本語");
  assert.equal(parts.at(-1), "overview.html");
  assert.equal(new URL("images/home.png", `https://app.example${url}`).pathname, url.replace("overview.html", "images/home.png"));
});

test("a comment names the file, the element and where it sits", () => {
  const el = { tag: "circle", selector: "svg > g > circle:nth-of-type(7)", label: "Sep 7: 4.9%", text: "", heading: "Install → trial", data: ['data-week="2026-09-07"'] };
  assert.equal(annotationText("tasks/r.html", el, "  Why the dip?\n"),
    'On `tasks/r.html`, at the <circle> "Sep 7: 4.9%" under "Install → trial" (`svg > g > circle:nth-of-type(7)`, data-week="2026-09-07"):\nWhy the dip?');
  const long = "x".repeat(100);
  assert.equal(annotationText("a.html", { tag: "h1", selector: "body > h1", label: "", text: long, heading: long, data: [] }, "c"),
    `On \`a.html\`, at the <h1> "${"x".repeat(79)}…" (\`body > h1\`):\nc`);
});
