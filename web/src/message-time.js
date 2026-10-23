/* Transcript timestamps. The formatters are built once: a transcript can hold
   thousands of bubbles, and constructing an Intl formatter per bubble is the
   costly part of formatting a time. */
const TIME = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" });
const FULL = new Intl.DateTimeFormat(undefined, { dateStyle: "full", timeStyle: "medium" });

// messageTime writes the clock time beside a role label, e.g. 12:32:44.
export function messageTime(ms) {
  return ms ? TIME.format(ms) : "";
}

// messageDate is the full date and time, for the label's tooltip.
export function messageDate(ms) {
  return ms ? FULL.format(ms) : "";
}
