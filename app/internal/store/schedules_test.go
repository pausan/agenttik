package store

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCalendarMigrationPreservesExistingSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.db")
	db := rawDatabase(t, path)
	at := slices.IndexFunc(migrations, func(sql string) bool { return strings.Contains(sql, "ADD COLUMN day_interval") })
	if at < 0 {
		t.Fatal("calendar migration missing")
	}
	if err := applyMigrations(db, 0, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO projects(id,name,path,created_at) VALUES(1,'project','/project',123);
INSERT INTO schedules(id,project_id,prompt,provider,model,every,at_minute,anchor_at,next_run_at,created_at)
VALUES(1,1,'morning check','fake','m','day',540,123,456,123);`)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, err := st.GetSchedule(1)
	if err != nil || s.Every != EveryDay || s.AtMinute != 540 || s.AnchorAt != 123 || s.NextRunAt != 456 || s.DayInterval != 1 || s.WeekdayMask != 0 {
		t.Fatalf("upgraded schedule = %+v, %v", s, err)
	}
}
