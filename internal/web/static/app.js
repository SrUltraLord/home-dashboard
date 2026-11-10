/* ES5 a propósito: debe correr en navegadores antiguos.
 *
 * La página nunca se recarga entera: el contenido nuevo se pide por XHR y solo
 * se sustituye si llegó bien. Así un corte de wifi (ERR_NETWORK_CHANGED, etc.)
 * no deja la pantalla del Kindle en una página de error; se queda lo último
 * que cargó y se reintenta. */
(function () {
  var app = document.getElementById('app');
  if (!app || !window.XMLHttpRequest) { return; }

  var cfg, loadedAt, homeTimer = null, busy = false;
  var current = location.pathname + location.search;

  function readCfg() {
    cfg = {
      now: parseInt(app.getAttribute('data-now'), 10) || 0,
      version: app.getAttribute('data-version') || '',
      kiosk: app.getAttribute('data-kiosk') !== 'false'
    };
    loadedAt = new Date().getTime();
  }

  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  // El reloj usa la hora del servidor + tiempo transcurrido: la zona horaria
  // que reporta el navegador del Kindle no es fiable.
  function tick() {
    var now = new Date(cfg.now + (new Date().getTime() - loadedAt));
    var hm = pad(now.getUTCHours()) + ':' + pad(now.getUTCMinutes());
    var clock = document.getElementById('clock');
    if (clock && clock.innerHTML !== hm) { clock.innerHTML = hm; }
    setTimeout(tick, 60000 - (now.getUTCSeconds() * 1000 + now.getUTCMilliseconds()) + 50);
  }

  // Ajusta el alto de las celdas para que el calendario llene la pantalla.
  var BASE = 62;
  function fit() {
    var grid = document.getElementById('grid');
    // El calendario cabe entero: sin barra de desplazamiento. Las listas de
    // pendientes pueden ser largas y sí necesitan desplazarse.
    document.documentElement.style.overflow = grid ? 'hidden' : '';
    if (!grid) { return; }
    var days = grid.getElementsByTagName('a'), i;
    for (i = 0; i < days.length; i++) { days[i].style.height = BASE + 'px'; }
    var vh = window.innerHeight || document.documentElement.clientHeight;
    var rest = document.body.offsetHeight - 6 * BASE;
    var h = Math.floor((vh - rest - 2) / 6);
    if (h < 40) { h = 40; }
    if (h > 150) { h = 150; }
    for (i = 0; i < days.length; i++) { days[i].style.height = h + 'px'; }
  }

  function get(url, ok, fail) {
    var x = new XMLHttpRequest(), done = false;
    function end(good) {
      if (done) { return; }
      done = true;
      if (good) { ok(x.responseText); } else if (fail) { fail(); }
    }
    x.open('GET', url, true);
    x.timeout = 15000;
    x.onreadystatechange = function () {
      if (x.readyState === 4) { end(x.status === 200 && !!x.responseText); }
    };
    x.onerror = x.ontimeout = function () { end(false); };
    try { x.send(null); } catch (e) { end(false); }
  }

  // Tras consultar otro día o mes, vuelve sola a la vista de hoy.
  function armHome() {
    if (homeTimer) { clearTimeout(homeTimer); homeTimer = null; }
    if (!cfg.kiosk) { homeTimer = setTimeout(function () { show('/', 0); }, 5 * 60000); }
  }

  // Carga url y sustituye el contenido; ante un fallo conserva lo que hay y
  // reintenta unas cuantas veces.
  function show(url, attempt) {
    if (busy && !attempt) { return; }
    busy = true;
    get(url, function (html) {
      var tmp = document.createElement('div');
      tmp.innerHTML = html;
      var next = tmp.querySelector ? tmp.querySelector('#app') : null;
      busy = false;
      if (!next) { return; }
      app.innerHTML = next.innerHTML;
      app.setAttribute('data-now', next.getAttribute('data-now'));
      app.setAttribute('data-version', next.getAttribute('data-version'));
      app.setAttribute('data-kiosk', next.getAttribute('data-kiosk'));
      current = url;
      try { history.replaceState(null, '', url); } catch (e) {}
      readCfg();
      fit();
      armHome();
      window.scrollTo(0, 0);
    }, function () {
      if ((attempt || 0) < 5) {
        setTimeout(function () { show(url, (attempt || 0) + 1); }, 4000);
      } else {
        busy = false;
      }
    });
  }

  // Actualiza solo cuando cambia algo visible (datos, día o aviso de desfase).
  function check() {
    get('/version?t=' + new Date().getTime(), function (v) {
      if (v !== cfg.version) { show(current, 0); }
    });
  }

  // Los enlaces internos se resuelven sin salir de la página.
  document.onclick = function (ev) {
    ev = ev || window.event;
    var a = ev.target || ev.srcElement;
    while (a && a.nodeName !== 'A') { a = a.parentNode; }
    if (!a || !a.getAttribute) { return; }
    var href = a.getAttribute('href');
    if (href === '#exit') {
      if (ev.preventDefault) { ev.preventDefault(); }
      tell('dashboard-exit');
      return false;
    }
    if (!href || href.charAt(0) !== '/') { return; }
    if (ev.preventDefault) { ev.preventDefault(); }
    show(href, 0);
    return false;
  };

  // Dentro de la app del Kindle (iframe): avisa al envoltorio de que la página
  // está viva y muestra el botón Salir, que ahí es la única forma de volver.
  var embedded = window.parent && window.parent !== window;
  function tell(what) {
    if (!embedded) { return; }
    try { window.parent.postMessage(what, '*'); } catch (e) {}
  }
  if (embedded) {
    document.body.className += ' embedded';
    tell('dashboard-ready');
    setInterval(function () { tell('dashboard-ready'); }, 60000);
  }

  readCfg();
  if (cfg.now) { tick(); }
  fit();
  armHome();
  window.onresize = fit;
  setInterval(check, 60000);
})();
