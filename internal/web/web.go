// Package web sirve el dashboard como HTML renderizado en el servidor,
// pensado para el navegador del Kindle (sin framework, JS opcional).
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"log"
	"net/http"
	"time"

	"home-dashboard/internal/agenda"
	"home-dashboard/internal/store"
)

//go:embed templates/*.html static/*
var assets embed.FS

// Server contiene las dependencias de los handlers.
type Server struct {
	Store *store.Store
	Loc   *time.Location
	Demo  bool
	// StaleAfter: sin sincronización correcta durante este tiempo se avisa en pantalla.
	StaleAfter time.Duration
	Now        func() time.Time

	tmpl    *template.Template
	stripes []byte
}

type page struct {
	Tab      string // "cal" | "tasks"
	Self     string
	Pending  int
	Cal      *agenda.Calendar
	Tasks    *agenda.TasksView
	Clock    string
	ClockSub string
	NowMs    int64 // hora local del servidor expresada como si fuera UTC
	Version  string
	Kiosk    bool // vista por defecto: no hay que volver a ella
}

// Handler construye el mux HTTP.
func (s *Server) Handler() (http.Handler, error) {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.Loc == nil {
		s.Loc = time.Local
	}
	t, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s.tmpl = t
	s.stripes = stripesPNG()

	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.calendar)
	mux.HandleFunc("GET /tasks", s.tasks)
	mux.HandleFunc("GET /version", s.version)
	mux.HandleFunc("GET /debug", s.debug)
	mux.HandleFunc("GET /debug/report", s.debugReport)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("GET /static/stripes.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Write(s.stripes)
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	return mux, nil
}

func (s *Server) now() time.Time { return s.Now().In(s.Loc) }

// stale indica si hay que avisar de que los datos no están al día.
func (s *Server) stale(snap store.Snapshot, now time.Time) bool {
	if s.Demo {
		return false
	}
	return snap.Err != "" || snap.SyncedAt.IsZero() || now.Sub(snap.SyncedAt) > s.StaleAfter
}

// versionAt cambia cuando cambia algo visible: datos, día o aviso de desfase.
func (s *Server) versionAt(snap store.Snapshot, now time.Time) string {
	v := s.Store.Hash() + "|" + now.Format(agenda.DateFmt)
	if s.stale(snap, now) {
		v += "|stale"
	}
	return v
}

func (s *Server) base(tab, self string, snap store.Snapshot, now time.Time) page {
	_, off := now.Zone()
	p := page{
		Tab:      tab,
		Self:     self,
		Pending:  len(snap.Tasks),
		Clock:    now.Format("15:04"),
		ClockSub: "Hora local",
		NowMs:    now.UnixMilli() + int64(off)*1000,
		Version:  s.versionAt(snap, now),
	}
	switch {
	case s.Demo:
		p.ClockSub = "Datos de ejemplo"
	case snap.SyncedAt.IsZero():
		p.ClockSub = "Sin sincronizar"
	case s.stale(snap, now):
		at := snap.SyncedAt.In(s.Loc)
		if at.Format(agenda.DateFmt) == now.Format(agenda.DateFmt) {
			p.ClockSub = "Sin sync desde " + at.Format("15:04")
		} else {
			p.ClockSub = "Sin sync desde " + agenda.WhenLabel(now, at)
		}
	}
	return p
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("plantilla %s: %v", name, err)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	buf.WriteTo(w)
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	today := agenda.Midnight(now)
	sel, month := today, agenda.FirstOfMonth(today)
	q := r.URL.Query()
	if d, err := time.ParseInLocation(agenda.DateFmt, q.Get("d"), s.Loc); err == nil {
		sel, month = d, agenda.FirstOfMonth(d)
	}
	if m, err := time.ParseInLocation(agenda.MonthFmt, q.Get("m"), s.Loc); err == nil {
		month = m
	}

	snap := s.Store.Get()
	cal := agenda.BuildCalendar(today, sel, month, snap.Events, snap.Tasks)
	p := s.base("cal", r.URL.RequestURI(), snap, now)
	p.Cal = &cal
	p.Kiosk = r.URL.RawQuery == ""
	s.render(w, "calendar.html", p)
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	snap := s.Store.Get()
	tv := agenda.BuildTasks(now, snap.Tasks)
	p := s.base("tasks", "/tasks", snap, now)
	p.Tasks = &tv
	p.Kiosk = true
	s.render(w, "tasks.html", p)
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, s.versionAt(s.Store.Get(), s.now()))
}

func (s *Server) debug(w http.ResponseWriter, r *http.Request) {
	log.Printf("debug: UA=%q remoto=%s", r.UserAgent(), r.RemoteAddr)
	s.render(w, "debug.html", map[string]any{"UA": r.UserAgent()})
}

// debugReport deja en el log lo que el navegador dice de sí mismo, para poder
// leerlo desde la Raspberry sin copiarlo a mano de la pantalla del Kindle.
func (s *Server) debugReport(w http.ResponseWriter, r *http.Request) {
	d := r.URL.Query().Get("d")
	if len(d) > 2000 {
		d = d[:2000]
	}
	log.Printf("debug-report: %q", d)
	w.WriteHeader(http.StatusNoContent)
}

// stripesPNG genera el mosaico de rayas diagonales de los días fuera de mes.
func stripesPNG() []byte {
	const n = 12
	img := image.NewGray(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			c := color.Gray{Y: 255}
			if (x+y)%n < 2 {
				c.Y = 0
			}
			img.SetGray(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
