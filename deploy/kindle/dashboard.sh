#!/bin/sh
# Name: Dashboard
# Author: David
#
# Scriptlet para el Kindle con jailbreak: va en /mnt/us/documents y aparece en
# la biblioteca como un libro. Abre el dashboard como app propia (sin la barra
# del navegador) y evita que el Kindle se suspenda mientras está en pantalla.
#
# La app es un WAF: /mnt/us/dashboard/waf se instala en
# /var/local/mesquite/dashboard y se registra en /var/local/appreg.db. Se
# reinstala en cada arranque del scriptlet, así que sobrevive a reinicios.

ID=com.david.dashboard
SRC=/mnt/us/dashboard/waf
DST=/var/local/mesquite/dashboard
DB=/var/local/appreg.db
LOG=/mnt/us/dashboard/dashboard.log

# Desligarse del lanzador de scriptlets, que si no devuelve la pantalla de
# inicio por encima de la app.
if [ "$1" != "--detached" ]; then
	if command -v setsid >/dev/null 2>&1; then
		setsid sh "$0" --detached >/dev/null 2>&1 </dev/null &
	else
		nohup sh "$0" --detached >/dev/null 2>&1 </dev/null &
	fi
	exit 0
fi
sleep 2

{
	echo "=== $(date) ==="
	if [ ! -f "$SRC/config.xml" ] || [ ! -f "$SRC/index.html" ]; then
		echo "faltan los archivos de la app en $SRC"
		exit 1
	fi

	mkdir -p "$DST"
	cp "$SRC/config.xml" "$SRC/index.html" "$DST/" && chmod 775 "$DST" && chmod 664 "$DST"/*
	[ -f /mnt/us/dashboard/appreg.db.bak ] || cp "$DB" /mnt/us/dashboard/appreg.db.bak
	sqlite3 "$DB" "
		INSERT OR IGNORE INTO handlerIds VALUES ('$ID');
		INSERT OR REPLACE INTO properties VALUES ('$ID','command','/usr/bin/mesquite -l $ID -c file://$DST/');
		INSERT OR REPLACE INTO properties VALUES ('$ID','unloadPolicy','unloadOnPause');
		INSERT OR REPLACE INTO properties VALUES ('$ID','framework','');
		INSERT OR REPLACE INTO associations VALUES ('$ID','application','none','false');"
	echo "registro rc=$?"

	lipc-set-prop com.lab126.powerd preventScreenSaver 1
	lipc-set-prop com.lab126.appmgrd start "app://$ID"
	echo "abrir app rc=$?"
} >>"$LOG" 2>&1

# Cuando la app deja de estar en pantalla (botón Salir, menú del sistema),
# el Kindle vuelve a poder suspenderse.
away=0
sleep 15
while [ "$away" -lt 2 ]; do
	sleep 5
	if [ "$(lipc-get-prop com.lab126.appmgrd activeApp 2>/dev/null)" = "$ID" ]; then
		away=0
	else
		away=$((away + 1))
	fi
done
lipc-set-prop com.lab126.powerd preventScreenSaver 0
echo "app cerrada $(date): suspensión restaurada" >>"$LOG"
