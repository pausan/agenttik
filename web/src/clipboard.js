/* writeClipboard puts text on the system clipboard. navigator.clipboard does
   not exist on plain HTTP, and Android's WebView refuses it on every origin,
   so a refusal falls back to the copy command. That needs a recent click but
   no secure context. It writes through the copy event instead of selecting
   a hidden field, which would take the focus from an open menu or the prompt
   and move the phone's keyboard. WebKit only enables the command with a
   selection unless beforecopy is cancelled. */
export async function writeClipboard(text, clipboard, doc) {
  try {
    await clipboard.writeText(text);
  } catch (cause) {
    if (!copyCommand(text, doc)) throw new Error("Could not copy to the clipboard", { cause });
  }
}

function copyCommand(text, doc) {
  const enable = (e) => e.preventDefault();
  const write = (e) => {
    e.clipboardData.setData("text/plain", text);
    e.preventDefault();
  };
  doc.addEventListener("beforecopy", enable);
  doc.addEventListener("copy", write);
  try {
    return doc.execCommand("copy");
  } finally {
    doc.removeEventListener("beforecopy", enable);
    doc.removeEventListener("copy", write);
  }
}
