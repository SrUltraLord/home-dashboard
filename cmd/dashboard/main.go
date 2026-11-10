// dashboard sirve la agenda eInk para el Kindle.
//
//	dashboard serve [--demo] [--addr :8080]
//	dashboard auth
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"home-dashboard/internal/agenda"
	"home-dashboard/internal/gsync"
	"home-dashboard/internal/store"
	"home-dashboard/internal/web"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "auth":
		err = auth(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		log.Fatal(err)
	}
}

// accessLog registra las peticiones de página (no el sondeo de /version ni
// los estáticos) para poder diagnosticar qué pide el Kindle.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; p != "/version" && p != "/healthz" && !strings.HasPrefix(p, "/static/") {
			log.Printf("http: %s %s %s desde %s UA=%q", r.Method, r.Host, r.URL.RequestURI(), r.RemoteAddr, r.UserAgent())
		}
		next.ServeHTTP(w, r)
	})
}

// connLog registra la primera conexión TCP de cada IP (como mucho una vez
// cada 10 minutos): distingue "no llega a la Raspberry" de "llega pero falla".
func connLog() func(net.Conn, http.ConnState) {
	var mu sync.Mutex
	seen := map[string]time.Time{}
	unused := map[net.Conn]time.Time{} // conexiones que aún no han enviado ninguna petición
	return func(c net.Conn, s http.ConnState) {
		host, _, _ := net.SplitHostPort(c.RemoteAddr().String())
		mu.Lock()
		defer mu.Unlock()
		switch s {
		case http.StateNew:
			unused[c] = time.Now()
			if time.Since(seen[host]) > 10*time.Minute {
				log.Printf("conn: nueva conexión desde %s", host)
			}
			seen[host] = time.Now()
		case http.StateActive:
			delete(unused, c)
		case http.StateClosed, http.StateHijacked:
			// El navegador del Kindle abre conexiones por adelantado; si se
			// cierran sin usarse, el siguiente toque puede dar error.
			if t, ok := unused[c]; ok {
				log.Printf("conn: %s cerrada sin peticiones tras %s", host, time.Since(t).Round(time.Second))
				delete(unused, c)
			}
		}
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: dashboard serve [flags] | dashboard auth [flags]")
	os.Exit(2)
}

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "home-dashboard")
}

func auth(args []string) error {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)
	dir := configDir()
	cred := fs.String("credentials", filepath.Join(dir, "credentials.json"), "cliente OAuth descargado de Google Cloud")
	token := fs.String("token", filepath.Join(dir, "token.json"), "dónde guardar el token")
	fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return gsync.Auth(ctx, *cred, *token)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dir := configDir()
	addr := fs.String("addr", ":8080", "dirección de escucha")
	demo := fs.Bool("demo", false, "usar datos de ejemplo en vez de Google")
	cred := fs.String("credentials", filepath.Join(dir, "credentials.json"), "cliente OAuth de Google")
	token := fs.String("token", filepath.Join(dir, "token.json"), "token OAuth")
	cache := fs.String("cache", filepath.Join(dir, "cache.json"), "cache del último snapshot")
	tz := fs.String("tz", "", "zona horaria IANA (por defecto, la del sistema)")
	interval := fs.Duration("interval", 5*time.Minute, "intervalo de sincronización")
	fs.Parse(args)

	loc := time.Local
	if *tz != "" {
		l, err := time.LoadLocation(*tz)
		if err != nil {
			return fmt.Errorf("zona horaria: %w", err)
		}
		loc = l
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st *store.Store
	if *demo {
		st = store.New("")
		now := time.Now().In(loc)
		ev, tk := agenda.Demo(now)
		st.Update(ev, tk, now)
	} else {
		st = store.New(*cache)
		syncer, err := gsync.New(ctx, *cred, *token, loc)
		if err != nil {
			// Se sigue sirviendo (con el cache, si lo hay) y el aviso sale en pantalla.
			log.Printf("Google no configurado: %v", err)
			st.Fail(err)
		} else {
			go syncer.Run(ctx, st, *interval)
		}
	}

	srv := &web.Server{Store: st, Loc: loc, Demo: *demo, StaleAfter: 3 * *interval}
	h, err := srv.Handler()
	if err != nil {
		return err
	}
	hs := &http.Server{
		Addr:    *addr,
		Handler: accessLog(h),
		// Holgados a propósito: Chrome (el navegador del Kindle) abre
		// conexiones especulativas y las usa minutos después; cerrarlas
		// pronto provoca ERR_CONNECTION_CLOSED al tocar un enlace.
		ReadHeaderTimeout: 10 * time.Minute,
		IdleTimeout:       10 * time.Minute,
		ConnState:         connLog(),
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	log.Printf("escuchando en %s (demo=%v, zona=%s)", *addr, *demo, loc)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
