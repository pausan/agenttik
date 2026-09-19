export const PRACTICE_REPO = "https://github.com/pausan/agenttik-helloworld.git";

const pause = "First wait about 30 seconds (run python -c \"import time; time.sleep(30)\", or use python3) so I can read and enqueue the next prompt. ";
const scope = "Work only in the agenttik-helloworld practice repository (find main.py in the project or its agenttik-helloworld subfolder). If it is missing, stop and ask me. ";
const commit = "Run the tests, then commit only this task's changes locally on the current tutorial branch with a descriptive message. Do not push. If Git needs an identity, ask me to configure it. Summarize the changes and test results. ";

export const TOUR_STEPS = [
  { id: "welcome", title: "Learn by building a calculator", paragraphs: [
    "Connect your first agent, clone a tiny Python project, and turn Hello World into a calculator while trying the task queue.",
    "You perform every action. Previous and Next are always available. Close this tour anytime and restart it from Settings → Help. Minimize it whenever you need more room.",
  ] },
  { id: "connect", title: "Set up your first agent", paragraphs: [
    "Open Settings → Providers. Under Subscriptions, choose a provider, install its CLI if needed, and sign in. Under API Providers, enter a key and click Enable. API tasks need Full access to run the tutorial commands and tests. Reopen Settings to refresh subscription status.",
    "One connected provider is enough to start. You can add more providers and subscriptions in Settings later, and choose a model for each task.",
  ] },
  { id: "project", title: "Create a practice project", paragraphs: [
    "Click Add project, choose Multiple Git Repos, and choose or create a new base folder such as ~/code/agenttik-practice. Paste this repository URL, name the project Quick Start, and click Add project to clone it locally.",
    "Before the public repo is uploaded, you can paste its absolute local path instead. Already cloned it? Choose Folder and select that checkout. Open the new project when cloning finishes.",
  ], repo: true },
  { id: "task", title: "Create your first task", paragraphs: [
    "In your practice project, click New task (or the + in the tab bar). Choose your connected provider and model below the prompt box. This task is a conversation with your agent, working in this project's folder.",
    "Use this same task for all four tutorial prompts so they share context. The button below each prompt fills the prompt box; you still choose when to Enqueue it.",
  ] },
  { id: "instructions", title: "Give your agent a way to work", paragraphs: [
    "This first prompt asks the agent to create a tutorial branch and an AGENTS.md file with simple project instructions: test each change and commit it locally. Read the prompt, copy it to the prompt area, then click Enqueue in the menu beside Send (or the main Enqueue button).",
    "A queued prompt starts when the project is idle. Each tutorial prompt begins with a 30-second pause so you have time to read and queue more work. Click Next after enqueuing.",
  ], prompt: scope + pause + "Check that the working tree is clean, then create and switch to a new branch named tutorial/calculator (use a fresh suffix if it exists). Create AGENTS.md with these instructions: keep this Python calculator simple; use Python's standard library; add and run unittest tests for changes; commit each completed change on the current tutorial branch; never push. Leave main.py as Hello World for now. " + commit },
  { id: "add", title: "Queue addition and subtraction", paragraphs: [
    "Copy this prompt and click Enqueue in the same task. You can write down the next idea while the agent works; it will continue with queued prompts after the current turn finishes.",
    "The queue appears below the conversation. Waiting prompts can be edited before they start. Stay in this task for the multiplication and division prompts too.",
  ], prompt: scope + pause + "On the tutorial branch created in the previous prompt, turn main.py into a simple command-line calculator that adds or subtracts two numbers. Keep the code importable and put command-line handling behind a main guard. Add unittest coverage for addition, subtraction, and invalid input. Include usage examples in README.md. " + commit },
  { id: "multiply", title: "Queue multiplication", paragraphs: [
    "Copy this prompt into the same task and click Enqueue again. You do not have to wait for addition and subtraction to finish.",
    "Small prompts and a commit per change make the history easier to review. Within a project, queued work waits for active turns to finish; the scheduler continues this task's queue before moving to other tasks.",
  ], prompt: scope + pause + "Extend the calculator in main.py with multiplication of two numbers. Preserve addition and subtraction, add multiplication tests, and update the usage examples. " + commit },
  { id: "divide", title: "Queue division", paragraphs: [
    "Copy the final calculator prompt and click Enqueue. The agent will build on the earlier changes and make another local commit.",
    "You can ask models to commit linearly on one branch, as here, or use a separate branch for each feature. Branches organize history; simultaneous edits need separate checkouts or Git worktrees to keep files apart.",
  ], prompt: scope + pause + "Extend the calculator in main.py with division of two numbers. Handle division by zero with a clear error. Add tests for division and zero division, preserve the other operations, and update README.md with examples for all four operations. " + commit },
  { id: "parallel", title: "Ask a question while work continues", paragraphs: [
    "Open a separate New task in this project. Copy this question and choose Send from the prompt menu to run it straightaway, alongside the calculator task. Send is available when this new task is idle, even if another task is working.",
    "This is useful for questions that do not change files: What is this project about? How is this feature built? How could we implement another operation? You choose how to work; read-only questions help avoid conflicting edits. Return to the calculator task to watch its queue.",
  ], prompt: "Read this project and explain what it is about, how the calculator is built so far, and how we could add exponentiation. Another task may still be changing the code. Answer only; do not modify files or create commits." },
  { id: "review", title: "Review changes and try the magic", paragraphs: [
    "Open Workspace → Commits to inspect the tutorial branch and each commit's changed files. Click a file to review its diff. Workspace → Changed shows edits that have not been committed yet; it may be empty after the agent commits.",
    "Once the queue finishes, try a manual commit: make a small README edit, save it, then stage that file in Changed. The sparkle button (Generate commit message) uses a model to suggest a message. Review it and click Commit yourself. Generating a message does not commit.",
    "On a phone, open Workspace from the header. Minimize this tour if it covers the controls.",
  ] },
  { id: "archive", title: "Keep tasks easy to find", paragraphs: [
    "When a task has finished and its queue is empty, use its Archive action. Archiving keeps the conversation and its history.",
    "Click the project to see its task list. Use Filter tasks (or Smart Search tasks if enabled) to find past work, including archived tasks. Restore an archived task when you want to continue it.",
  ] },
  { id: "finish", title: "Take this workflow to your projects", paragraphs: [
    "Each project has its own independent queue. Work can run in several projects at once without waiting for another project's queue. Use separate folders or checkouts so those jobs do not edit the same files.",
    "Start with a small task, enqueue the next ideas, review each change, and keep useful checkpoints with local commits. Add more providers in Settings when you need them.",
    "You have the basics. Revisit this Quick Start Tour from Settings → Help whenever you like.",
  ] },
];

export function connectedAccounts(providers) {
  return providers.filter((p) => p.available).flatMap((p) =>
    (p.accounts || []).filter((a) => a.signed_in).map((a) => `${p.display_name} · ${a.alias}`));
}

export function insertTourPrompt(owner, prompt) {
  if (!owner || !prompt || owner.draft?.trim() || owner.imageUploads) return false;
  owner.draft = prompt;
  return true;
}
