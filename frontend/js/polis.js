(function () {
  'use strict';

  function esc(s) {
    return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }
  function safePolisUrl(u) {
    return (u && String(u).startsWith('/')) ? 'https://polisen.se' + u : '#';
  }

  var TYPE_COLORS = {
    'Trafikolycka': '#d29922', 'Trafikolycka, personskada': '#d29922',
    'Trafikbrott': '#e3b341', 'Trafikkontroll': '#e3b341', 'Rattfylleri': '#e0823d',
    'Stöld': '#bc8cff', 'Stöld/inbrott': '#bc8cff', 'Inbrott': '#b083f0', 'Rån': '#9e7bff',
    'Misshandel': '#ff7b72', 'Misshandel, grov': '#ff7b72', 'Våld/hot mot tjänsteman': '#ec8e7b',
    'Brand': '#f78166', 'Bråk': '#ec8e7b', 'Skottlossning': '#ff5d52',
    'Narkotikabrott': '#7c6cff', 'Vapenlagen': '#6cb6ff',
    'Sammanfattning natt': '#54aeff', 'Sammanfattning kväll och natt': '#54aeff',
    'Försvunnen person': '#2dd4a0', 'Räddningsinsats': '#56d364', 'Djur': '#57ab5a',
    'Övrigt': '#6b82a0'
  };
  var FALLBACK_COLORS = ['#4fa3e8', '#2dd4a0', '#bc8cff', '#d29922', '#ff7b72', '#38b6ff', '#56d364'];

  function typeColor(t) {
    if (TYPE_COLORS[t]) return TYPE_COLORS[t];
    var h = 0;
    for (var i = 0; i < t.length; i++) h = (h * 31 + t.charCodeAt(i)) >>> 0;
    return FALLBACK_COLORS[h % FALLBACK_COLORS.length];
  }

  var fmt = new Intl.NumberFormat('sv-SE');
  var MONTHS = ['jan', 'feb', 'mar', 'apr', 'maj', 'jun', 'jul', 'aug', 'sep', 'okt', 'nov', 'dec'];

  Chart.defaults.color = '#6b82a0';
  Chart.defaults.borderColor = 'rgba(27,41,64,.6)';
  Chart.defaults.font.family = "'DM Mono', monospace";
  Chart.defaults.font.size = 10;

  var events = [];      // all fetched events
  var names = {};       // county code -> name
  var PAGE = 25;
  var shown = PAGE;

  function countUp(el, target, dur) {
    var start = null;
    function frame(ts) {
      if (!start) start = ts;
      var t = Math.min((ts - start) / dur, 1);
      el.textContent = fmt.format(Math.round(target * (1 - Math.pow(1 - t, 3))));
      if (t < 1) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }

  function evTime(iso) {
    if (!iso) return { day: '', time: '' };
    var d = new Date(iso);
    return {
      day: d.getDate() + ' ' + MONTHS[d.getMonth()],
      time: String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0')
    };
  }

  /* ── Feed rendering ──────────────────────────────── */
  function filtered() {
    var c = document.getElementById('f-county').value;
    var t = document.getElementById('f-type').value;
    return events.filter(function (e) {
      return (!c || e.county === c) && (!t || e.type === t);
    });
  }

  function renderFeed() {
    var list = filtered();
    var feed = document.getElementById('feed');
    feed.innerHTML = '';
    document.getElementById('f-count').textContent = list.length + ' händelser';
    document.getElementById('feed-empty').hidden = list.length > 0;

    list.slice(0, shown).forEach(function (e, i) {
      var a = document.createElement('a');
      a.className = 'event';
      a.href = safePolisUrl(e.url);
      a.target = '_blank';
      a.rel = 'noopener';
      a.style.setProperty('--ec', typeColor(e.type));
      a.style.animationDelay = Math.min(i, 12) * 30 + 'ms';
      var t = evTime(e.event_time);
      var locExtra = names[e.county] && e.location.indexOf(' län') === -1 && names[e.county] !== e.location ? ' · ' + esc(names[e.county]) : '';
      a.innerHTML =
        '<div class="ev-time"><span class="day">' + esc(t.day) + '</span>' + esc(t.time) + '</div>' +
        '<div class="ev-body">' +
          '<div class="ev-head">' +
            '<span class="ev-type">' + esc(e.type) + '</span>' +
            '<span class="ev-loc">' + esc(e.location) + locExtra + '</span>' +
          '</div>' +
          '<div class="ev-summary">' + esc(e.summary) + '</div>' +
        '</div>' +
        '<svg class="ev-arrow" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M7 17L17 7M9 7h8v8"/></svg>';
      feed.appendChild(a);
    });
    document.getElementById('load-more').hidden = list.length <= shown;
  }

  document.getElementById('f-county').addEventListener('change', function () { shown = PAGE; renderFeed(); });
  document.getElementById('f-type').addEventListener('change', function () { shown = PAGE; renderFeed(); });
  document.getElementById('load-more').addEventListener('click', function () { shown += PAGE; renderFeed(); });

  /* ── Charts ──────────────────────────────────────── */
  function buildTypeChart() {
    var counts = {};
    events.forEach(function (e) { counts[e.type] = (counts[e.type] || 0) + 1; });
    var top = Object.keys(counts)
      .map(function (t) { return { type: t, n: counts[t] }; })
      .sort(function (a, b) { return b.n - a.n; })
      .slice(0, 10);

    new Chart(document.getElementById('type-chart'), {
      type: 'bar',
      data: {
        labels: top.map(function (r) { return r.type; }),
        datasets: [{
          data: top.map(function (r) { return r.n; }),
          backgroundColor: top.map(function (r) { return typeColor(r.type) + 'cc'; }),
          borderRadius: 4,
          barThickness: 16
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { display: false },
          tooltip: { backgroundColor: '#0f1726', borderColor: '#2a3f5f', borderWidth: 1, padding: 10,
                     bodyFont: { family: "'DM Mono', monospace" } } },
        scales: {
          x: { grid: { color: 'rgba(27,41,64,.35)' }, ticks: { precision: 0 } },
          y: { grid: { display: false },
               ticks: { font: { family: "'DM Sans', sans-serif", size: 11 }, color: '#a8bdd4' } }
        }
      }
    });
  }

  function buildCapitaChart(summary) {
    var rows = summary
      .filter(function (r) { return r.population && r.police_events_7d; })
      .map(function (r) {
        return { name: r.name, v: r.police_events_7d / r.population * 100000 };
      })
      .sort(function (a, b) { return b.v - a.v; });

    new Chart(document.getElementById('capita-chart'), {
      type: 'bar',
      data: {
        labels: rows.map(function (r) { return r.name; }),
        datasets: [{
          data: rows.map(function (r) { return Math.round(r.v * 10) / 10; }),
          backgroundColor: rows.map(function (_, i) {
            var t = i / Math.max(rows.length - 1, 1);
            return 'rgba(' + Math.round(247 - t * 191) + ',' + Math.round(129 + t * 53) + ',' + Math.round(102 + t * 130) + ',.85)';
          }),
          borderRadius: 4,
          barThickness: 10
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { display: false },
          tooltip: { backgroundColor: '#0f1726', borderColor: '#2a3f5f', borderWidth: 1, padding: 10,
                     bodyFont: { family: "'DM Mono', monospace" },
                     callbacks: { label: function (c) { return ' ' + c.parsed.x + ' per 100k inv'; } } } },
        scales: {
          x: { grid: { color: 'rgba(27,41,64,.35)' } },
          y: { grid: { display: false },
               ticks: { font: { family: "'DM Sans', sans-serif", size: 10 }, color: '#a8bdd4', autoSkip: false } }
        }
      }
    });
  }

  /* ── Load ────────────────────────────────────────── */
  Promise.all([
    fetch('/api/v1/policeevents').then(function (r) { return r.json(); }),
    fetch('/api/v1/lan/summary').then(function (r) { return r.json(); })
  ]).then(function (res) {
    events = res[0].data || [];
    var summary = res[1].data || [];
    summary.forEach(function (r) { names[r.code] = r.name; });

    /* hero band */
    var total7d = 0, latest = null, latestName = '';
    summary.forEach(function (r) { total7d += r.police_events_7d || 0; });
    events.forEach(function (e) {
      if (e.event_time && (!latest || e.event_time > latest)) { latest = e.event_time; latestName = e.type; }
    });
    countUp(document.getElementById('ev-7d'), total7d, 1500);
    if (latest) {
      var t = evTime(latest);
      document.getElementById('ev-latest').textContent = 'senaste: ' + latestName + ' ' + t.day + ' ' + t.time;
    }

    var counts = {};
    events.forEach(function (e) { counts[e.type] = (counts[e.type] || 0) + 1; });
    var topType = Object.keys(counts).sort(function (a, b) { return counts[b] - counts[a]; })[0];
    if (topType) {
      document.getElementById('top-type').textContent = topType;
      document.getElementById('top-type-n').textContent = counts[topType] + ' av ' + events.length + ' senaste';
    }

    var capita = summary
      .filter(function (r) { return r.population && r.police_events_7d; })
      .map(function (r) { return { name: r.name, v: r.police_events_7d / r.population * 100000 }; })
      .sort(function (a, b) { return b.v - a.v; })[0];
    if (capita) {
      document.getElementById('top-capita').textContent = capita.name;
      document.getElementById('top-capita-n').textContent = capita.v.toFixed(1).replace('.', ',') + ' per 100k (7 dgr)';
    }

    /* filter dropdowns */
    var cs = document.getElementById('f-county');
    summary.forEach(function (r) {
      var o = document.createElement('option');
      o.value = r.code; o.textContent = r.name;
      cs.appendChild(o);
    });
    var ts = document.getElementById('f-type');
    Object.keys(counts).sort(function (a, b) { return counts[b] - counts[a]; }).forEach(function (t) {
      var o = document.createElement('option');
      o.value = t; o.textContent = t + ' (' + counts[t] + ')';
      ts.appendChild(o);
    });

    renderFeed();
    buildTypeChart();
    buildCapitaChart(summary);
  }).catch(function (err) {
    document.getElementById('feed-empty').textContent = 'Kunde inte nå API:et — prova att ladda om sidan.';
    document.getElementById('feed-empty').hidden = false;
    console.error('API load failed:', err);
  });
})();
