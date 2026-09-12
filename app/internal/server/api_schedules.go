package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func scheduleID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return 0, badRequest("invalid schedule id")
	}
	return id, nil
}

// maxScheduleRuns bounds the run list a schedule view draws. A schedule that
// has fired for a year has thousands; the page shows the recent ones and says
// so rather than sending the lot.
const maxScheduleRuns = 200

func (s *Server) listSchedules(c *fiber.Ctx) error {
	name := c.Query("window", defaultWindow)
	d, ok := windows[name]
	if !ok {
		return badRequest("unknown window %q", name)
	}
	f := store.ScheduleFilter{
		ProjectID:   int64(c.QueryInt("project_id")),
		Query:       c.Query("q"),
		Limit:       c.QueryInt("limit", 200),
		ExcludeDone: !c.QueryBool("include_done", true),
	}
	if d > 0 {
		f.Since = time.Now().Add(-d).UnixMilli()
	}
	schedules, err := s.store.ListSchedules(f)
	if err != nil {
		return err
	}
	return c.JSON(schedules)
}

// checkRecurrence is the one place a clock is read, since creating a schedule
// and changing one later take the same three fields and must not drift apart.
func checkRecurrence(every string, intervalMinutes, atMinute int64) error {
	switch every {
	case store.EveryInterval, store.EveryDay, store.EveryWeek, store.EveryMonth:
	default:
		return badRequest("unknown recurrence %q", every)
	}
	if every == store.EveryInterval && intervalMinutes < 1 {
		return badRequest("interval must be at least one minute")
	}
	if atMinute < 0 || atMinute > 24*60-1 {
		return badRequest("time of day must be within the day")
	}
	return nil
}

// createSchedule turns the prompt in the box into a repeating one. Everything
// it needs is already on screen — the prompt, the project, the model — so the
// dialog only asks for the recurrence and the number of runs.
func (s *Server) createSchedule(c *fiber.Ctx) error {
	var body struct {
		ProjectID int64  `json:"project_id"`
		Prompt    string `json:"prompt"`
		Provider  string `json:"provider"`
		// AccountID is which subscription the job's runs go on. Absent is the
		// provider's default.
		AccountID       *int64 `json:"account_id"`
		Model           string `json:"model"`
		Effort          string `json:"effort"`
		Permission      string `json:"permission"`
		Title           string `json:"title"`
		Every           string `json:"every"`
		IntervalMinutes int64  `json:"interval_minutes"`
		AtMinute        int64  `json:"at_minute"`
		Remaining       int64  `json:"remaining"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		return badRequest("prompt is required")
	}
	project, err := s.store.GetProject(body.ProjectID)
	if err != nil {
		return err
	}
	provider, ok := s.registry.Get(body.Provider)
	if !ok {
		return badRequest("unknown provider %q", body.Provider)
	}
	if body.Model == "" {
		body.Model = provider.Models()[0].ID
	}
	if err := checkRecurrence(body.Every, body.IntervalMinutes, body.AtMinute); err != nil {
		return err
	}
	// Below -1 is -1: there is one way to say forever, and 0 is what the
	// counter reaches on its own rather than something to be asked for.
	if body.Remaining < -1 {
		body.Remaining = -1
	}

	// The first line is the name the job is drawn with straight away; a
	// better one is fetched behind it, below, so no dialog has to ask for one.
	title, named := strings.TrimSpace(body.Title), true
	if title == "" {
		title, named = scheduleTitle(prompt), false
	}
	accountID, err := s.chooseAccount(provider.Name(), body.AccountID, store.SystemAccount, true)
	if err != nil {
		return err
	}
	sched := &store.Schedule{
		ProjectID:       project.ID,
		Title:           title,
		Prompt:          prompt,
		Provider:        provider.Name(),
		AccountID:       accountID,
		Model:           body.Model,
		Effort:          body.Effort,
		Permission:      string(agent.Permission(body.Permission).Valid()),
		Every:           body.Every,
		IntervalMinutes: body.IntervalMinutes,
		AtMinute:        body.AtMinute,
		AnchorAt:        time.Now().UnixMilli(),
		Remaining:       body.Remaining,
	}
	sched.NextRunAt = runner.FirstRun(sched, time.Now()).UnixMilli()
	if err := s.store.CreateSchedule(sched); err != nil {
		return err
	}
	// A job created with a name of its own is not guessed at; one named from
	// its first line is, and the guess arrives a few seconds later.
	if !named {
		s.runner.NameSchedule(sched)
	}
	full, err := s.store.GetSchedule(sched.ID)
	if err != nil {
		return err
	}
	s.runner.Hub().Publish(runner.ProjectTopic(full.ProjectID), runner.Event{
		ProjectID: full.ProjectID, Event: agent.Event{Type: runner.EventScheduleChanged}})
	return c.Status(fiber.StatusCreated).JSON(scheduleDetail{Schedule: full, Runs: []store.ScheduleRun{}})
}

// scheduleDetail is everything the schedule page draws: the schedule and the
// runs it has spawned or skipped, newest first.
type scheduleDetail struct {
	Schedule *store.Schedule     `json:"schedule"`
	Runs     []store.ScheduleRun `json:"runs"`
}

func (s *Server) getSchedule(c *fiber.Ctx) error {
	id, err := scheduleID(c)
	if err != nil {
		return err
	}
	sched, err := s.store.GetSchedule(id)
	if err != nil {
		return err
	}
	runs, err := s.store.ListScheduleRuns(sched.ID, maxScheduleRuns)
	if err != nil {
		return err
	}
	return c.JSON(scheduleDetail{Schedule: sched, Runs: runs})
}

// updateSchedule takes any subset. The prompt, the model, the clock and the
// counter are all plain fields the view can change whenever — every 15
// minutes to every morning, infinite to five, one model to another — because
// none of what a schedule repeats is a decision made once.
func (s *Server) updateSchedule(c *fiber.Ctx) error {
	var body struct {
		Title     *string `json:"title"`
		Remaining *int64  `json:"remaining"`
		Paused    *bool   `json:"paused"`
		Done      *bool   `json:"done"`

		// The prompt and the model are as changeable as the clock: a
		// schedule worth keeping is rarely worth remaking over a typo or a
		// model that turned out to be the wrong one.
		Prompt    *string `json:"prompt"`
		Provider  *string `json:"provider"`
		AccountID *int64  `json:"account_id"`
		Model     string  `json:"model"`
		Effort    string  `json:"effort"`

		// The three recurrence fields move together, since they are one
		// answer: every names the form, and the other two carry it.
		Every           *string `json:"every"`
		IntervalMinutes int64   `json:"interval_minutes"`
		AtMinute        int64   `json:"at_minute"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	id, err := scheduleID(c)
	if err != nil {
		return err
	}
	sched, err := s.store.GetSchedule(id)
	if err != nil {
		return err
	}
	if body.Title != nil {
		if title := strings.TrimSpace(*body.Title); title != "" {
			if err := s.store.SetScheduleTitle(sched.ID, title); err != nil {
				return err
			}
		}
	}
	if body.Prompt != nil {
		prompt := strings.TrimSpace(*body.Prompt)
		if prompt == "" {
			return badRequest("prompt is required")
		}
		if err := s.store.SetSchedulePrompt(sched.ID, prompt); err != nil {
			return err
		}
	}
	// Provider, model and effort move together for the same reason the clock's
	// three fields do: an effort belongs to a model and a model to a provider,
	// so a model picked without its provider could name one the provider on
	// the row has never heard of.
	if body.Provider != nil {
		provider, ok := s.registry.Get(*body.Provider)
		if !ok {
			return badRequest("unknown provider %q", *body.Provider)
		}
		model := body.Model
		if model == "" {
			model = provider.Models()[0].ID
		}
		accountID, err := s.chooseAccount(provider.Name(), body.AccountID, sched.AccountID,
			provider.Name() != sched.Provider)
		if err != nil {
			return err
		}
		if err := s.store.SetScheduleModel(sched.ID, provider.Name(), accountID, model, body.Effort); err != nil {
			return err
		}
	}
	if body.Remaining != nil {
		if err := s.store.SetScheduleRemaining(sched.ID, *body.Remaining); err != nil {
			return err
		}
	}
	// A new clock and a resume both restart the cadence from now, so the next
	// run is booked once, after whichever came in has been applied.
	rebook := false
	if body.Every != nil {
		if err := checkRecurrence(*body.Every, body.IntervalMinutes, body.AtMinute); err != nil {
			return err
		}
		// The weekday and the day of the month come off the anchor, which is
		// the moment the form was chosen — so choosing it again moves it, and
		// a weekly schedule dragged from 09:00 to 10:00 keeps its Tuesday.
		anchor := sched.AnchorAt
		if *body.Every != sched.Every {
			anchor = time.Now().UnixMilli()
		}
		if err := s.store.SetScheduleRecurrence(sched.ID, *body.Every,
			body.IntervalMinutes, body.AtMinute, anchor); err != nil {
			return err
		}
		sched.Every, sched.IntervalMinutes = *body.Every, body.IntervalMinutes
		sched.AtMinute, sched.AnchorAt = body.AtMinute, anchor
		rebook = true
	}
	if body.Paused != nil {
		if err := s.store.SetSchedulePaused(sched.ID, *body.Paused); err != nil {
			return err
		}
		// Resuming restarts the cadence from now. A pause holds runs back; it
		// does not save them up to be fired all at once on release.
		if !*body.Paused {
			rebook = true
		}
	}
	if rebook {
		if err := s.store.SetScheduleNextRun(sched.ID,
			runner.FirstRun(sched, time.Now()).UnixMilli()); err != nil {
			return err
		}
	}
	if body.Done != nil {
		// Archiving pauses too, so nothing hidden keeps starting sessions —
		// which also means a restored schedule comes back paused.
		if err := s.store.SetScheduleDone(sched.ID, *body.Done); err != nil {
			return err
		}
	}
	full, err := s.store.GetSchedule(sched.ID)
	if err != nil {
		return err
	}
	s.runner.Hub().Publish(runner.ProjectTopic(full.ProjectID), runner.Event{
		ProjectID: full.ProjectID, Event: agent.Event{Type: runner.EventScheduleChanged}})
	return c.JSON(full)
}

// runScheduleNow forces one run outside the schedule's own clock. It runs
// even if a previous fire of this schedule is still going, and never spends
// the counter — see runner.RunScheduleNow. enqueue picks the same two words
// the prompt bar offers: Send starts beside whatever else the project is
// running, Enqueue waits for the project to be free.
func (s *Server) runScheduleNow(c *fiber.Ctx) error {
	var body struct {
		Enqueue bool `json:"enqueue"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	id, err := scheduleID(c)
	if err != nil {
		return err
	}
	if err := s.runner.RunScheduleNow(id, body.Enqueue); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// deleteSchedule drops the schedule and its run history. The sessions it
// spawned are ordinary sessions and are left alone.
func (s *Server) deleteSchedule(c *fiber.Ctx) error {
	id, err := scheduleID(c)
	if err != nil {
		return err
	}
	sched, err := s.store.GetSchedule(id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteSchedule(id); err != nil {
		return err
	}
	s.runner.Hub().Publish(runner.ProjectTopic(sched.ProjectID), runner.Event{
		ProjectID: sched.ProjectID, Event: agent.Event{Type: runner.EventScheduleChanged}})
	return c.SendStatus(fiber.StatusNoContent)
}

// reorderSchedules records the order the sidebar was dragged into.
func (s *Server) reorderSchedules(c *fiber.Ctx) error {
	id, err := projectID(c)
	if err != nil {
		return err
	}
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if err := s.store.ReorderSchedules(id, body.IDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// scheduleTitle is the name a job is drawn with until the model behind it
// answers (runner.NameSchedule), and the one it keeps if that never happens.
// Every run carries the same name, which is what makes forty of them readable
// in the Sessions list; the runs are told apart by their timestamps and by the
// job number they share.
func scheduleTitle(prompt string) string {
	title := strings.TrimSpace(strings.SplitN(prompt, "\n", 2)[0])
	const max = 80
	if len([]rune(title)) > max {
		title = string([]rune(title)[:max]) + "…"
	}
	if title == "" {
		title = "Untitled schedule"
	}
	return title
}
