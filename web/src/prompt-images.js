const references = /!\[Attached image\]\((\/api\/attachments\/[a-f0-9]{64}\.(?:png|jpg|gif|webp))\)/g;

export function promptImages(draft = "") {
  return Array.from(draft.matchAll(references), (match) => ({ reference: match[0], url: match[1] }));
}

export function promptText(draft = "") {
  return draft.replace(new RegExp(`(?:\\n\\n)?${references.source}`, "g"), "");
}

export function withImages(text, images) {
  return [text, ...images.map((image) => image.reference)].filter(Boolean).join("\n\n");
}

export function clipboardImages(data) {
  return Array.from(data?.items || [])
    .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
    .map((item) => item.getAsFile()).filter(Boolean);
}

export async function uploadImage(file) {
  if (file.size > 4 * 1024 * 1024) throw new Error("Each pasted image must be 4 MiB or smaller.");
  const res = await fetch("/api/attachments", { method: "POST", body: file });
  if (res.status === 401) {
    window.location.reload();
    throw new Error("Signed out");
  }
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Could not save image");
  return { url: data.url, reference: `![Attached image](${data.url})` };
}
