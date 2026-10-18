import test from "node:test";
import assert from "node:assert/strict";
import { projectQueueAction } from "./project-queue.js";

for (const paused of [false, true]) {
  for (const running of [false, true]) {
    for (const queued of [false, true]) {
      test(`queue action: paused=${paused}, running=${running}, queued=${queued}`, () => {
        const action = projectQueueAction({
          queue_paused: paused,
          recent_sessions: [{ status: running ? "running" : "idle", queue_count: queued ? 2 : 0 }],
        });
        const visible = paused ? running || queued : running;
        assert.equal(!!action, visible);
        if (visible) {
          assert.equal(action.paused, !paused);
          assert.equal(action.label, paused ? "Resume project queue" : "Pause project queue");
        }
      });
    }
  }
}

test("a running scheduled task offers pause", () => {
  assert.equal(projectQueueAction({ schedules: [{ running: true }] }).paused, true);
  assert.equal(projectQueueAction(null), null);
});
