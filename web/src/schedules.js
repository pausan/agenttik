/* What the dialog opens on. A quarter of an hour is the common case, and
   whatever was entered last replaces it, so the second schedule is one
   click. */
export const DEFAULT_SCHEDULE = { every: "interval", hours: 0, minutes: 15, at: "09:00", days: 2, weekdayMask: 62, remaining: -1 };

/* The forms a schedule can take. The dialog asks for one and the schedule
   view offers the same forms, since a clock can be changed after the fact. */
export const EVERY = [
  { label: "Hours and minutes", value: "interval" },
  { label: "Day", value: "day" },
  { label: "Every N days", value: "days" },
  { label: "Selected weekdays", value: "weekdays" },
  { label: "Week", value: "week" },
  { label: "Month", value: "month" },
];

// Sunday is bit 0, matching Go's time.Weekday; show Monday first.
export const WEEKDAYS = [
  { label: "Mon", value: 2 },
  { label: "Tue", value: 4 },
  { label: "Wed", value: 8 },
  { label: "Thu", value: 16 },
  { label: "Fri", value: 32 },
  { label: "Sat", value: 64 },
  { label: "Sun", value: 1 },
];

export function scheduleRecurrence(form) {
  return {
    every: form.every,
    interval_minutes: form.every === "interval" ? intervalMinutes(form) : 0,
    at_minute: form.every === "interval" ? 0 : atMinute(form.at),
    day_interval: form.every === "days" ? Math.min(36500, Math.max(1, Math.trunc(Number(form.days) || 1))) : 1,
    weekday_mask: form.every === "weekdays" ? form.weekdayMask : 0,
  };
}

/* The dialog asks for hours and minutes; the server stores the one number. */
export function intervalMinutes(form) {
  return Math.max(1, Number(form.hours || 0) * 60 + Number(form.minutes || 0));
}

/* "09:30" as minutes past midnight, local — the clock on the wall is what
   "every day at 9" is read off. */
export function atMinute(at) {
  const [h, m] = String(at || "").split(":");
  return Math.min(24 * 60 - 1, Math.max(0, (Number(h) || 0) * 60 + (Number(m) || 0)));
}

/* "09:30" from minutes past local midnight, the way the field wants it. */
function clock(atMinute) {
  return String(Math.floor(atMinute / 60)).padStart(2, "0") +
    ":" + String(atMinute % 60).padStart(2, "0");
}

export function scheduleLabel(schedule) {
  if (schedule.every === "pinned") return "Pinned task";
  if (schedule.every === "interval") {
    const h = Math.floor(schedule.interval_minutes / 60);
    const m = schedule.interval_minutes % 60;
    return "Every " + [h ? h + "h" : "", m ? m + "m" : ""].filter(Boolean).join(" ");
  }
  if (schedule.every === "days") return `Every ${schedule.day_interval} days at ${clock(schedule.at_minute)}`;
  if (schedule.every === "weekdays") {
    const days = WEEKDAYS.filter((day) => schedule.weekday_mask & day.value).map((day) => day.label);
    return `${days.join(", ")} at ${clock(schedule.at_minute)}`;
  }
  return `Every ${schedule.every} at ${clock(schedule.at_minute)}`;
}

/* A schedule as the dialog's fields, for the view that edits them. Only the
   half of the clock its form uses is stored — the other is 0 — so switching
   form offers what the dialog would have, not midnight or no interval. */
export function scheduleForm(schedule) {
  const interval = schedule.every === "interval";
  return {
    every: schedule.every,
    hours: interval ? Math.floor(schedule.interval_minutes / 60) : DEFAULT_SCHEDULE.hours,
    minutes: interval ? schedule.interval_minutes % 60 : DEFAULT_SCHEDULE.minutes,
    at: interval ? DEFAULT_SCHEDULE.at : clock(schedule.at_minute),
    days: schedule.every === "days" ? schedule.day_interval : DEFAULT_SCHEDULE.days,
    weekdayMask: schedule.every === "weekdays" ? schedule.weekday_mask : DEFAULT_SCHEDULE.weekdayMask,
  };
}

