package agenda

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestBuildCalendarGrid(t *testing.T) {
	cases := []struct {
		name        string
		month       time.Time
		first, last string // primera y última celda
	}{
		{"empieza en domingo", day(2026, time.November, 1), "2026-10-26", "2026-12-06"},
		{"empieza en lunes", day(2026, time.June, 1), "2026-06-01", "2026-07-12"},
		{"febrero no bisiesto", day(2026, time.February, 1), "2026-01-26", "2026-03-08"},
		{"cambio de año", day(2026, time.December, 1), "2026-11-30", "2027-01-10"},
	}
	for _, c := range cases {
		cal := BuildCalendar(day(2026, time.November, 10), day(2026, time.November, 10), c.month, nil, nil)
		if got := cal.Weeks[0][0].Date; got != c.first {
			t.Errorf("%s: primera celda = %s, quiero %s", c.name, got, c.first)
		}
		if got := cal.Weeks[5][6].Date; got != c.last {
			t.Errorf("%s: última celda = %s, quiero %s", c.name, got, c.last)
		}
	}
}

func TestBuildCalendarMarksAndLabels(t *testing.T) {
	today := day(2026, time.November, 10)
	events := []Event{
		{"2026-11-10", "16:00", "Tarde"}, {"2026-11-10", "08:30", "Daily"}, {"2026-11-10", "", "Festivo"},
		{"2026-11-10", "11:00", "Otra"}, {"2026-11-12", "09:00", "Sprint"},
	}
	tasks := []Task{{"1", "a", "2026-11-12"}, {"2", "b", ""}}
	cal := BuildCalendar(today, today, today, events, tasks)

	if cal.MonthTitle != "Noviembre" || cal.Year != 2026 {
		t.Errorf("título = %s %d", cal.MonthTitle, cal.Year)
	}
	if cal.PrevMonth != "2026-10" || cal.NextMonth != "2026-12" {
		t.Errorf("prev/next = %s / %s", cal.PrevMonth, cal.NextMonth)
	}
	if cal.SelNum != "10" || cal.SelLabel != "Hoy, martes 10 de noviembre de 2026" {
		t.Errorf("selección = %q %q", cal.SelNum, cal.SelLabel)
	}
	want := []string{"Festivo", "08:30 Daily", "11:00 Otra", "16:00 Tarde"}
	if len(cal.SelEvents) != len(want) {
		t.Fatalf("eventos = %v", cal.SelEvents)
	}
	for i := range want {
		if cal.SelEvents[i] != want[i] {
			t.Errorf("evento %d = %q, quiero %q", i, cal.SelEvents[i], want[i])
		}
	}

	// Semana del 9 al 15 de noviembre: fila 2.
	c10, c12 := cal.Weeks[2][1], cal.Weeks[2][3]
	if c10.Date != "2026-11-10" || !c10.Today || !c10.Selected || len(c10.Events) != 3 || c10.Pending {
		t.Errorf("celda del 10 = %+v", c10)
	}
	if c12.Date != "2026-11-12" || len(c12.Events) != 1 || !c12.Pending || c12.Today {
		t.Errorf("celda del 12 = %+v", c12)
	}
	if !cal.Weeks[0][0].Out || cal.Weeks[2][1].Out {
		t.Error("marca de fuera de mes incorrecta")
	}

	other := BuildCalendar(today, day(2026, time.November, 14), today, nil, nil)
	if other.SelLabel != "Sábado 14 de noviembre de 2026" {
		t.Errorf("etiqueta = %q", other.SelLabel)
	}
}

func TestWhenLabel(t *testing.T) {
	today := day(2026, time.November, 10)
	cases := map[string]time.Time{
		"mañana":      day(2026, time.November, 11),
		"ayer":        day(2026, time.November, 9),
		"hace 3 días": day(2026, time.November, 7),
		"vie 13":      day(2026, time.November, 13),
		"mié 2 dic":   day(2026, time.December, 2),
		"dom 10 ene":  day(2027, time.January, 10),
	}
	for want, d := range cases {
		if got := WhenLabel(today, d); got != want {
			t.Errorf("WhenLabel(%s) = %q, quiero %q", d.Format(DateFmt), got, want)
		}
	}
}

func TestBuildTasks(t *testing.T) {
	today := day(2026, time.November, 10)
	v := BuildTasks(today, []Task{
		{"1", "próxima", "2026-11-13"}, {"2", "hoy", "2026-11-10"}, {"3", "sin fecha", ""},
		{"4", "vieja", "2026-11-07"}, {"5", "ayer", "2026-11-09"},
	})
	if v.Count != 5 || v.Title != "Pendientes" {
		t.Errorf("cabecera = %d %q", v.Count, v.Title)
	}
	if want := "2 atrasadas · 1 para hoy · 1 próximas · 1 sin fecha"; v.Sub != want {
		t.Errorf("sub = %q, quiero %q", v.Sub, want)
	}
	if len(v.Groups) != 4 {
		t.Fatalf("grupos = %d", len(v.Groups))
	}
	late := v.Groups[0]
	if !late.Late || late.Items[0] != (TaskItem{"vieja", "hace 3 días"}) || late.Items[1] != (TaskItem{"ayer", "ayer"}) {
		t.Errorf("atrasadas = %+v", late)
	}
	if v.Groups[1].Title != "Hoy" || v.Groups[1].Items[0].When != "" {
		t.Errorf("hoy = %+v", v.Groups[1])
	}

	empty := BuildTasks(today, nil)
	if empty.Sub != "Todo al día" || len(empty.Groups) != 0 {
		t.Errorf("vacío = %+v", empty)
	}
	if one := BuildTasks(today, []Task{{"1", "x", ""}}); one.Title != "Pendiente" {
		t.Errorf("singular = %q", one.Title)
	}
}
