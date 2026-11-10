#!/bin/sh
# Name: Instalar Jobman
# Author: David
#
# El descargador de KPM va muy lento en este Kindle, así que este scriptlet
# baja el paquete de Jobman con curl (con reintentos y reanudación) y lo
# instala desde un repositorio local file://. Al final restaura el
# repositorio oficial. Log: /mnt/us/install-jobman.log

if [ "$1" != "--worker" ]; then
	cp "$0" /tmp/install-jobman-worker.sh
	if command -v setsid >/dev/null 2>&1; then
		setsid sh /tmp/install-jobman-worker.sh --worker >/dev/null 2>&1 </dev/null &
	else
		nohup sh /tmp/install-jobman-worker.sh --worker >/dev/null 2>&1 </dev/null &
	fi
	exit 0
fi

WORK=/var/local/install-jobman.log
OUT=/mnt/us/install-jobman.log
FBINK=/mnt/us/libkh/bin/fbink
KPM=/var/local/kmc/kindlehf/bin/kpm
OFFICIAL=https://repo.kindlemodding.org/manifest.v2.json
PKG=https://repo.kindlemodding.org/packages/jobman/artifacts/jobman_1.2.2_kindlehf.kpkg
SIZE=6701984
REPO=/mnt/us/kmc/localrepo
FILE=$REPO/jobman_1.2.2_kindlehf.kpkg
T0=$(date +%s)

publish() { cp "$WORK" "$OUT" 2>/dev/null; }
say() {
	echo ">> [$(( $(date +%s) - T0 ))s] $1" >>"$WORK"
	publish
	[ -x "$FBINK" ] && "$FBINK" -q -pm "$1" >/dev/null 2>&1
}
fsize() { wc -c <"$FILE" 2>/dev/null | tr -d ' ' || echo 0; }

# Ejecuta "$@" con límite de $1 segundos y un latido cada 30 s.
run() {
	limit=$1
	shift
	echo "--- $* (límite ${limit}s)" >>"$WORK"
	"$@" >>"$WORK" 2>&1 </dev/null &
	pid=$!
	n=0
	while kill -0 "$pid" 2>/dev/null; do
		sleep 1
		n=$((n + 1))
		if [ $((n % 30)) -eq 0 ]; then
			echo "   ...${n}s, descargado $(fsize) de $SIZE bytes" >>"$WORK"
			publish
		fi
		if [ "$n" -ge "$limit" ]; then
			echo "   TIEMPO AGOTADO tras ${n}s" >>"$WORK"
			kill "$pid" 2>/dev/null
			sleep 1
			kill -9 "$pid" 2>/dev/null
			break
		fi
	done
	wait "$pid" 2>/dev/null
	rc=$?
	echo "   rc=$rc tras ${n}s" >>"$WORK"
	publish
	return $rc
}

echo "=== $(date) ===" >"$WORK"
# Sin esto el Kindle se suspende a los pocos minutos y corta la wifi.
lipc-set-prop com.lab126.powerd preventScreenSaver 1 >>"$WORK" 2>&1
mkdir -p "$REPO"

# 1. Descarga con reanudación: hasta 40 intentos de 5 minutos.
say "Jobman: descargando paquete"
i=0
while [ "$(fsize)" != "$SIZE" ] && [ "$i" -lt 40 ]; do
	i=$((i + 1))
	[ "$(fsize)" -gt "$SIZE" ] 2>/dev/null && rm -f "$FILE"
	run 320 curl -4 -L -sS -C - --speed-limit 1500 --speed-time 40 -m 300 \
		-w 'intento: http=%{http_code} vel=%{speed_download}B/s\n' -o "$FILE" "$PKG"
	say "Jobman: $(( $(fsize) * 100 / SIZE ))% descargado (intento $i)"
done

if [ "$(fsize)" != "$SIZE" ]; then
	say "Jobman: DESCARGA INCOMPLETA, reintenta luego"
	lipc-set-prop com.lab126.powerd preventScreenSaver 0 >>"$WORK" 2>&1
	publish
	exit 1
fi

# 2. Repositorio local que apunta al archivo descargado.
cat >"$REPO/manifest.v2.json" <<EOF
{
  "manifest_version": 2,
  "id": "localjobman",
  "name": "Jobman local",
  "description": "Copia local de Jobman descargada con curl",
  "packages": {
    "jobman": {
      "name": "Jobman",
      "author": "KindleTweaks",
      "description": "Kindle jailbreak manager",
      "artifacts": [
        {
          "url": "file://$FILE",
          "version": [1, 2, 2],
          "dependencies": [],
          "supported_platforms": ["kindlehf"]
        }
      ]
    }
  }
}
EOF

# 3. Instalar solo desde el repositorio local (se quita el oficial un momento
#    para que KPM no elija la descarga lenta).
say "Jobman: instalando"
run 60 "$KPM" list-repo
run 60 "$KPM" remove-repo kindlemodding
run 60 "$KPM" add-repo "file://$REPO/manifest.v2.json"
run 120 "$KPM" update
run 600 "$KPM" -d -y install jobman
INSTALL_RC=$?

# 4. Dejar KPM como estaba.
say "Jobman: restaurando repositorio oficial"
run 60 "$KPM" remove-repo localjobman
j=0
until run 90 "$KPM" add-repo "$OFFICIAL" || [ "$j" -ge 4 ]; do
	j=$((j + 1))
	sleep 5
done
run 120 "$KPM" update
run 60 "$KPM" list-repo

lipc-set-prop com.lab126.powerd preventScreenSaver 0 >>"$WORK" 2>&1
{
	echo "--- documents"
	ls -la /mnt/us/documents 2>&1
	echo "--- paquetes kpm"
	ls -la /mnt/us/kmc/kpm /mnt/us/kmc/kpm/packages 2>&1
} >>"$WORK"

if [ "$INSTALL_RC" -eq 0 ]; then
	rm -f "$FILE"
	say "Jobman: TERMINADO (instalado)"
else
	say "Jobman: TERMINADO CON ERROR rc=$INSTALL_RC"
fi
