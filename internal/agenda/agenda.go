// Package agenda contiene la lógica pura de la agenda: cuadrícula del mes,
// agrupación de tareas y etiquetas en español. No hace E/S.
package agenda

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DateFmt es el formato de las claves de fecha (día civil en hora local).
const DateFmt = "2006-01-02"

// MonthFmt es el formato del parámetro de mes.
const MonthFmt = "2006-01"

// Event es un evento ya proyectado sobre un día concreto.
type Event struct {
	Date  string `json:"date"`           // YYYY-MM-DD
	Time  string `json:"time,omitempty"` // HH:MM; vacío = todo el día
	Title string `json:"title"`
}

// Task es una tarea pendiente.
type Task struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Due   string `json:"due,omitempty"` // YYYY-MM-DD; vacío = sin fecha
}

var meses = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
var dias = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
var diasCortos = [...]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Midnight devuelve las 00:00 del día de t en su zona.
func Midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// AddDays suma días civiles (inmune a cambios de horario).
func AddDays(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+n, 0, 0, 0, 0, t.Location())
}

// FirstOfMonth devuelve el día 1 del mes de t.
func FirstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func civilDays(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// Cell es una celda de la cuadrícula mensual.
type Cell struct {
	Date     string
	Day      int
	Out      bool // fuera del mes mostrado
	Today    bool
	Selected bool
	Weekend  bool
	Events   []struct{} // una marca por evento, máximo 3
	Pending  bool
}

// Calendar es el modelo de la página de calendario.
type Calendar struct {
	MonthTitle string
	Year       int
	PrevMonth  string // YYYY-MM
	NextMonth  string
	Selected   string // YYYY-MM-DD
	Weeks      [6][7]Cell

	SelNum    string   // "02"
	SelLabel  string   // "Hoy, martes 10 de noviembre de 2026"
	SelEvents []string // "08:30 Daily del equipo"
}

// BuildCalendar arma la cuadrícula de 6 semanas (lunes a domingo) del mes
// indicado, con el día seleccionado resaltado.
func BuildCalendar(today, sel, month time.Time, events []Event, tasks []Task) Calendar {
	today, sel, month = Midnight(today), Midnight(sel), FirstOfMonth(month)
	todayKey, selKey := today.Format(DateFmt), sel.Format(DateFmt)

	evCount := map[string]int{}
	for _, e := range events {
		evCount[e.Date]++
	}
	pending := map[string]bool{}
	for _, t := range tasks {
		if t.Due != "" {
			pending[t.Due] = true
		}
	}

	c := Calendar{
		MonthTitle: capitalize(meses[month.Month()-1]),
		Year:       month.Year(),
		PrevMonth:  month.AddDate(0, -1, 0).Format(MonthFmt),
		NextMonth:  month.AddDate(0, 1, 0).Format(MonthFmt),
		Selected:   selKey,
	}

	start := AddDays(month, -((int(month.Weekday()) + 6) % 7))
	for i := 0; i < 42; i++ {
		d := AddDays(start, i)
		k := d.Format(DateFmt)
		n := evCount[k]
		if n > 3 {
			n = 3
		}
		c.Weeks[i/7][i%7] = Cell{
			Date:     k,
			Day:      d.Day(),
			Out:      d.Month() != month.Month(),
			Today:    k == todayKey,
			Selected: k == selKey,
			Weekend:  i%7 >= 5,
			Events:   make([]struct{}, n),
			Pending:  pending[k],
		}
	}

	c.SelNum = fmt.Sprintf("%02d", sel.Day())
	dia := dias[sel.Weekday()]
	if selKey == todayKey {
		dia = "Hoy, " + dia
	} else {
		dia = capitalize(dia)
	}
	c.SelLabel = fmt.Sprintf("%s %d de %s de %d", dia, sel.Day(), meses[sel.Month()-1], sel.Year())

	var day []Event
	for _, e := range events {
		if e.Date == selKey {
			day = append(day, e)
		}
	}
	sort.SliceStable(day, func(i, j int) bool { return day[i].Time < day[j].Time })
	for _, e := range day {
		if e.Time == "" {
			c.SelEvents = append(c.SelEvents, e.Title)
		} else {
			c.SelEvents = append(c.SelEvents, e.Time+" "+e.Title)
		}
	}
	return c
}

// TaskItem es una fila de la lista de pendientes.
type TaskItem struct {
	Title string
	When  string // etiqueta relativa; vacía si el grupo no la muestra
}

// TaskGroup es un bloque de la lista (Atrasadas, Hoy, ...).
type TaskGroup struct {
	Title string
	Late  bool
	Items []TaskItem
}

// TasksView es el modelo de la página de pendientes.
type TasksView struct {
	Count  int
	Title  string
	Sub    string
	Groups []TaskGroup
}

// WhenLabel devuelve la etiqueta relativa de una fecha respecto a hoy.
func WhenLabel(today, d time.Time) string {
	diff := civilDays(d) - civilDays(today)
	switch {
	case diff == 1:
		return "mañana"
	case diff == -1:
		return "ayer"
	case diff < 0:
		return fmt.Sprintf("hace %d días", -diff)
	}
	s := fmt.Sprintf("%s %d", diasCortos[d.Weekday()], d.Day())
	if d.Month() != today.Month() || d.Year() != today.Year() {
		s += " " + meses[d.Month()-1][:3]
	}
	return s
}

// BuildTasks agrupa las tareas pendientes en Atrasadas / Hoy / Próximos días /
// Sin fecha, ordenadas por vencimiento.
func BuildTasks(today time.Time, tasks []Task) TasksView {
	today = Midnight(today)
	todayKey := today.Format(DateFmt)

	sorted := append([]Task(nil), tasks...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Due < sorted[j].Due })

	late := TaskGroup{Title: "Atrasadas", Late: true}
	now := TaskGroup{Title: "Hoy"}
	next := TaskGroup{Title: "Próximos días"}
	nodate := TaskGroup{Title: "Sin fecha"}
	for _, t := range sorted {
		if t.Due == "" {
			nodate.Items = append(nodate.Items, TaskItem{Title: t.Title})
			continue
		}
		d, err := time.ParseInLocation(DateFmt, t.Due, today.Location())
		if err != nil {
			nodate.Items = append(nodate.Items, TaskItem{Title: t.Title})
			continue
		}
		switch {
		case t.Due < todayKey:
			late.Items = append(late.Items, TaskItem{t.Title, WhenLabel(today, d)})
		case t.Due == todayKey:
			now.Items = append(now.Items, TaskItem{Title: t.Title})
		default:
			next.Items = append(next.Items, TaskItem{t.Title, WhenLabel(today, d)})
		}
	}

	v := TasksView{Count: len(sorted), Title: "Pendientes", Sub: "Todo al día"}
	if v.Count == 1 {
		v.Title = "Pendiente"
	}
	if v.Count > 0 {
		var parts []string
		if n := len(late.Items); n > 0 {
			parts = append(parts, fmt.Sprintf("%d atrasadas", n))
		}
		parts = append(parts, fmt.Sprintf("%d para hoy", len(now.Items)), fmt.Sprintf("%d próximas", len(next.Items)))
		if n := len(nodate.Items); n > 0 {
			parts = append(parts, fmt.Sprintf("%d sin fecha", n))
		}
		v.Sub = strings.Join(parts, " · ")
	}
	for _, g := range []TaskGroup{late, now, next, nodate} {
		if len(g.Items) > 0 {
			v.Groups = append(v.Groups, g)
		}
	}
	return v
}

// Demo devuelve los datos de ejemplo del prototipo, relativos a hoy.
func Demo(today time.Time) ([]Event, []Task) {
	today = Midnight(today)
	k := func(o int) string { return AddDays(today, o).Format(DateFmt) }
	events := []Event{
		{k(0), "08:30", "Daily del equipo"}, {k(0), "11:00", "Revisión de arquitectura"}, {k(0), "16:00", "Entrevista técnica"},
		{k(1), "10:00", "Mercado de Santa Clara"}, {k(3), "09:00", "Planificación de sprint"}, {k(3), "15:30", "Demo con el cliente"},
		{k(5), "18:00", "Píldora tech: diseño de APIs"}, {k(8), "14:00", "Taller de diagramas C4"},
		{k(-2), "09:30", "Retro del sprint"}, {k(12), "12:00", "Almuerzo con el equipo"},
	}
	tasks := []Task{
		{"d1", "Actualizar diagrama de contenedores", k(0)}, {"d2", "Responder correo de seguridad", k(0)},
		{"d3", "Subir imagen Docker a staging", k(0)}, {"d4", "Comprar fuente de poder", k(1)},
		{"d5", "Enviar resumen de la demo", k(3)}, {"d6", "Ensayar la píldora tech", k(5)},
		{"d7", "Renovar certificado TLS", k(9)}, {"d8", "Cerrar tickets de SonarQube", k(-1)},
		{"d9", "Documentar el endpoint de reportes", k(-3)},
	}
	return events, tasks
}
