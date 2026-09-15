export const TOUR_KEY = "agenttik.quickStart.v1";

// A missing marker only opens the tour in an empty workspace. Existing users
// can start it from Help. Storage has the app's instance/profile/private scope.
export function readTour(storage, empty) {
  try {
    const saved = JSON.parse(storage.getItem(TOUR_KEY));
    if (saved && typeof saved.open === "boolean") {
      return { open: saved.open, step: typeof saved.step === "string" ? saved.step : "welcome" };
    }
  } catch {}
  return { open: empty, step: "welcome" };
}

export function saveTour(storage, state) {
  try { storage.setItem(TOUR_KEY, JSON.stringify(state)); } catch {}
}
