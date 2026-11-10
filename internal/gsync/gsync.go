// Package gsync lee Google Calendar y Google Tasks (solo lectura) y mantiene
// actualizado el store.
package gsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"home-dashboard/internal/agenda"
	"home-dashboard/internal/store"
)

func oauthConfig(credPath string) (*oauth2.Config, error) {
	b, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("leyendo credenciales OAuth: %w", err)
	}
	return google.ConfigFromJSON(b, calendar.CalendarReadonlyScope, tasks.TasksReadonlyScope)
}

// Auth ejecuta el flujo OAuth de aplicación de escritorio: levanta un
// servidor en localhost, imprime la URL de autorización y guarda el token.
func Auth(ctx context.Context, credPath, tokenPath string) error {
	cfg, err := oauthConfig(credPath)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	sb := make([]byte, 16)
	if _, err := rand.Read(sb); err != nil {
		return err
	}
	state := hex.EncodeToString(sb)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state inválido", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			fmt.Fprintln(w, "Autorización rechazada:", e)
			done <- result{err: errors.New("autorización rechazada: " + e)}
			return
		}
		fmt.Fprintln(w, "Listo. Ya puedes cerrar esta pestaña.")
		done <- result{code: q.Get("code")}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	// prompt=consent garantiza que Google devuelva refresh token.
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	fmt.Printf("Abre esta URL en el navegador y autoriza el acceso:\n\n%s\n\nEsperando...\n", url)

	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if res.err != nil {
		return res.err
	}
	tok, err := cfg.Exchange(ctx, res.code)
	if err != nil {
		return fmt.Errorf("canjeando el código: %w", err)
	}
	if tok.RefreshToken == "" {
		return errors.New("Google no devolvió refresh token")
	}
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(tokenPath, b, 0o600); err != nil {
		return err
	}
	fmt.Println("Token guardado en", tokenPath)
	return nil
}

// Syncer consulta Google periódicamente.
type Syncer struct {
	cal   *calendar.Service
	tasks *tasks.Service
	loc   *time.Location
}

// New crea el cliente a partir de credentials.json y token.json.
func New(ctx context.Context, credPath, tokenPath string, loc *time.Location) (*Syncer, error) {
	cfg, err := oauthConfig(credPath)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("leyendo token (ejecuta `dashboard auth`): %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, fmt.Errorf("token inválido: %w", err)
	}
	client := option.WithHTTPClient(cfg.Client(ctx, &tok))
	cal, err := calendar.NewService(ctx, client)
	if err != nil {
		return nil, err
	}
	ts, err := tasks.NewService(ctx, client)
	if err != nil {
		return nil, err
	}
	return &Syncer{cal: cal, tasks: ts, loc: loc}, nil
}

// Run sincroniza de inmediato y luego cada interval, hasta que ctx termine.
func (s *Syncer) Run(ctx context.Context, st *store.Store, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		now := time.Now().In(s.loc)
		events, tks, err := s.Fetch(cctx, now)
		cancel()
		if err != nil {
			log.Printf("sync: %v", err)
			st.Fail(err)
		} else {
			if err := st.Update(events, tks, now); err != nil {
				log.Printf("sync: guardando cache: %v", err)
			}
			log.Printf("sync: %d eventos, %d tareas", len(events), len(tks))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Fetch descarga eventos (del mes anterior a dos meses adelante) y tareas pendientes.
func (s *Syncer) Fetch(ctx context.Context, now time.Time) ([]agenda.Event, []agenda.Task, error) {
	month := agenda.FirstOfMonth(now)
	from, to := month.AddDate(0, -1, -7), month.AddDate(0, 3, 7)

	var events []agenda.Event
	err := s.cal.CalendarList.List().Context(ctx).Pages(ctx, func(l *calendar.CalendarList) error {
		for _, c := range l.Items {
			if !c.Selected || c.Hidden || c.Deleted {
				continue
			}
			err := s.cal.Events.List(c.Id).Context(ctx).
				SingleEvents(true).OrderBy("startTime").MaxResults(2500).
				TimeMin(from.Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339)).
				Pages(ctx, func(evs *calendar.Events) error {
					for _, e := range evs.Items {
						events = append(events, s.expand(e)...)
					}
					return nil
				})
			if err != nil {
				return fmt.Errorf("calendario %q: %w", c.Summary, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var out []agenda.Task
	err = s.tasks.Tasklists.List().Context(ctx).Pages(ctx, func(l *tasks.TaskLists) error {
		for _, tl := range l.Items {
			err := s.tasks.Tasks.List(tl.Id).Context(ctx).
				ShowCompleted(false).ShowHidden(false).MaxResults(100).
				Pages(ctx, func(ts *tasks.Tasks) error {
					for _, t := range ts.Items {
						if t.Title == "" || t.Status == "completed" || t.Deleted {
							continue
						}
						due := ""
						if len(t.Due) >= 10 { // la API solo guarda la fecha
							due = t.Due[:10]
						}
						out = append(out, agenda.Task{ID: t.Id, Title: t.Title, Due: due})
					}
					return nil
				})
			if err != nil {
				return fmt.Errorf("lista %q: %w", tl.Title, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return events, out, nil
}

// expand proyecta un evento de Google sobre cada día que ocupa.
func (s *Syncer) expand(e *calendar.Event) []agenda.Event {
	if e.Status == "cancelled" || e.Start == nil || e.End == nil {
		return nil
	}
	for _, a := range e.Attendees {
		if a.Self && a.ResponseStatus == "declined" {
			return nil
		}
	}
	title := e.Summary
	if title == "" {
		title = "(Sin título)"
	}

	if e.Start.DateTime == "" { // todo el día; la fecha de fin es exclusiva
		start, err1 := time.ParseInLocation(agenda.DateFmt, e.Start.Date, s.loc)
		end, err2 := time.ParseInLocation(agenda.DateFmt, e.End.Date, s.loc)
		if err1 != nil || err2 != nil {
			return nil
		}
		var out []agenda.Event
		for d := start; d.Before(end) && len(out) < 62; d = agenda.AddDays(d, 1) {
			out = append(out, agenda.Event{Date: d.Format(agenda.DateFmt), Title: title})
		}
		return out
	}

	start, err1 := time.Parse(time.RFC3339, e.Start.DateTime)
	end, err2 := time.Parse(time.RFC3339, e.End.DateTime)
	if err1 != nil || err2 != nil {
		return nil
	}
	start, end = start.In(s.loc), end.In(s.loc)
	out := []agenda.Event{{Date: start.Format(agenda.DateFmt), Time: start.Format("15:04"), Title: title}}
	// Días siguientes de un evento que cruza la medianoche (sin contar un fin exacto a las 00:00).
	for d := agenda.AddDays(start, 1); d.Before(end) && len(out) < 62; d = agenda.AddDays(d, 1) {
		out = append(out, agenda.Event{Date: d.Format(agenda.DateFmt), Title: title})
	}
	return out
}
