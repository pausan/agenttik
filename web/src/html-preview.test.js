import assert from "node:assert/strict";
import { test } from "node:test";
import { htmlPreviewURL } from "./html-preview.js";

test("relative asset resolution keeps routing scope and URL-encodes filenames", () => {
  const url = new URL(htmlPreviewURL({ projectID: 7, path: "docs/mock up/overview.html" }, { remote: "office", profile: "work" }), "https://app.example");
  assert.equal(new URL("images/photo%20%231.png", url).pathname, "/api/html/office/work/projects/7/preview/-/docs/mock%20up/images/photo%20%231.png");
  assert.equal(new URL("../styles/main.css", url).pathname, "/api/html/office/work/projects/7/preview/-/docs/styles/main.css");
});

test("absolute documents use their own directory, including Unicode paths", () => {
  const url = htmlPreviewURL({ projectID: 2, path: "/tmp/maquetas/日本語/overview.html" });
  const parts = url.split("/");
  assert.equal(Buffer.from(parts.at(-2), "base64url").toString(), "/tmp/maquetas/日本語");
  assert.equal(parts.at(-1), "overview.html");
  assert.equal(new URL("images/home.png", `https://app.example${url}`).pathname, url.replace("overview.html", "images/home.png"));
});
