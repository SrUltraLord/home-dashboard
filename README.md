# home-dashboard

Agenda eInk de solo lectura para el Kindle: un servidor Go en la Raspberry lee
Google Calendar y Google Tasks y sirve HTML pensado para el navegador del Kindle.

- Calendario: `http://192.168.100.244:8080/`
- Pendientes: `http://192.168.100.244:8080/tasks`
- Sonda del navegador: `http://192.168.100.244:8080/debug`

## Desarrollo

```bash
make test    # vet + tests
make demo    # datos de ejemplo en http://127.0.0.1:8090
make deploy  # compila para arm64 y reinicia el servicio en la Raspberry
make logs
```

El servicio es una unidad systemd **de usuario** en la Raspberry
(`~/.config/systemd/user/home-dashboard.service`); no hace falta sudo.

## Conectar Google

1. En Google Cloud Console: crear un proyecto y habilitar **Google Calendar API** y **Google Tasks API**.
2. Pantalla de consentimiento OAuth: tipo externo, y **publicar la app ("In production")**.
   En modo "Testing" el token caduca a los 7 días.
3. Crear credenciales → ID de cliente OAuth → tipo **Aplicación de escritorio**.
   Guardar el JSON como `~/.config/home-dashboard/credentials.json`.
4. En el portátil:

   ```bash
   go run ./cmd/dashboard auth
   ```

   Abre la URL que imprime, autoriza (solo lectura) y se guarda `token.json`.
5. Subirlos a la Raspberry y salir del modo demo:

   ```bash
   make secrets
   ```

Mientras exista `~/.config/home-dashboard/env` con `DASHBOARD_FLAGS=--demo` en la
Raspberry, se muestran datos de ejemplo.

## Notas

- El reloj usa la hora del servidor; la zona horaria del navegador del Kindle no es fiable.
- La página se recarga sola solo cuando cambian los datos o el día (consulta `/version` cada minuto).
- Límites de la API de Tasks: el vencimiento no tiene hora y las tareas asignadas desde Docs/Chat no aparecen.
