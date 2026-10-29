(function () {
  'use strict';

  /* Projection — MUST match the generation of the county SVG:
     lon 11.11–24.16, lat 55.35–69.04 → viewBox 500×820, padding 16 */
  var LON_MIN = 11.11, LON_MAX = 24.16, LAT_MIN = 55.35, LAT_MAX = 69.04;
  var W = 500, H = 820, PAD = 16;
  function project(lat, lon) {
    return {
      x: PAD + (lon - LON_MIN) / (LON_MAX - LON_MIN) * (W - 2 * PAD),
      y: H - PAD - (lat - LAT_MIN) / (LAT_MAX - LAT_MIN) * (H - 2 * PAD)
    };
  }

  /* Cold→warm scale, anchored to sensible Swedish extremes */
  var STOPS = [
    [-25, [124, 108, 255]], [-10, [56, 182, 255]], [0, [121, 192, 255]],
    [8, [45, 212, 160]], [15, [227, 179, 65]], [22, [247, 129, 102]], [30, [255, 93, 82]]
  ];
  function tempColor(t) {
    if (t <= STOPS[0][0]) return 'rgb(' + STOPS[0][1].join(',') + ')';
    for (var i = 1; i < STOPS.length; i++) {
      if (t <= STOPS[i][0]) {
        var f = (t - STOPS[i - 1][0]) / (STOPS[i][0] - STOPS[i - 1][0]);
        var c = STOPS[i - 1][1].map(function (a, j) { return Math.round(a + (STOPS[i][1][j] - a) * f); });
        return 'rgb(' + c.join(',') + ')';
      }
    }
    return 'rgb(' + STOPS[STOPS.length - 1][1].join(',') + ')';
  }

  var fmt1 = function (t) { return t.toFixed(1).replace('.', ',') + '°C'; };
  var COUNTY_NAMES = {};

  Chart.defaults.color = '#6b82a0';
  Chart.defaults.font.family = "'DM Mono', monospace";
  Chart.defaults.font.size = 10;

  var tooltip = document.getElementById('tooltip');

  function showTip(e, st) {
    document.getElementById('tt-name').textContent = st.name;
    var tEl = document.getElementById('tt-temp');
    tEl.textContent = st.temperature !== null ? fmt1(st.temperature) : '—';
    tEl.style.color = st.temperature !== null ? tempColor(st.temperature) : 'var(--dim)';
    var when = st.measured_at ? new Date(st.measured_at) : null;
    document.getElementById('tt-meta').textContent =
      (COUNTY_NAMES[st.county] || st.county) +
      ' · ' + Math.round(st.height_m || 0) + ' m ö.h.' +
      (when ? ' · kl ' + String(when.getHours()).padStart(2, '0') + ':' + String(when.getMinutes()).padStart(2, '0') : '');
    tooltip.classList.add('visible');
    moveTip(e);
  }
  function moveTip(e) {
    var tw = tooltip.offsetWidth || 180, th = tooltip.offsetHeight || 90;
    var x = e.clientX + 16, y = e.clientY + 16;
    if (x + tw > window.innerWidth - 10) x = e.clientX - tw - 10;
    if (y + th > window.innerHeight - 10) y = e.clientY - th - 10;
    tooltip.style.left = x + 'px';
    tooltip.style.top = y + 'px';
  }
  function hideTip() { tooltip.classList.remove('visible'); }

  Promise.all([
    fetch('/api/v1/weather').then(function (r) { return r.json(); }),
    fetch('/api/v1/lan/summary').then(function (r) { return r.json(); })
  ]).then(function (res) {
    var stations = (res[0].data || []).filter(function (s) { return s.temperature !== null; });
    (res[1].data || []).forEach(function (r) { COUNTY_NAMES[r.code] = r.name; });

    var svg = document.getElementById('sweden-map');
    var temps = stations.map(function (s) { return s.temperature; });
    var min = Math.min.apply(null, temps), max = Math.max.apply(null, temps);
    var mean = temps.reduce(function (a, b) { return a + b; }, 0) / temps.length;

    /* dots */
    var NS = 'http://www.w3.org/2000/svg';
    stations.forEach(function (st, i) {
      var p = project(st.lat, st.lon);
      if (p.x < -10 || p.x > W + 10 || p.y < -10 || p.y > H + 10) return;
      var c = document.createElementNS(NS, 'circle');
      c.setAttribute('cx', p.x.toFixed(1));
      c.setAttribute('cy', p.y.toFixed(1));
      c.setAttribute('r', 4);
      c.setAttribute('class', 'station');
      c.setAttribute('fill', tempColor(st.temperature));
      c.style.animationDelay = (0.3 + (i % 40) * 0.02) + 's';
      c.addEventListener('mouseenter', function (e) { c.setAttribute('r', 6); showTip(e, st); });
      c.addEventListener('mousemove', moveTip);
      c.addEventListener('mouseleave', function () { c.setAttribute('r', 4); hideTip(); });
      svg.appendChild(c);
    });

    /* legend bounds */
    document.getElementById('legend-min').textContent = fmt1(min);
    document.getElementById('legend-max').textContent = fmt1(max);

    /* extremes */
    var sorted = stations.slice().sort(function (a, b) { return b.temperature - a.temperature; });
    var warm = sorted[0], cold = sorted[sorted.length - 1];
    document.getElementById('warm-temp').textContent = fmt1(warm.temperature);
    document.getElementById('warm-name').textContent = warm.name;
    document.getElementById('warm-county').textContent = COUNTY_NAMES[warm.county] || warm.county;
    document.getElementById('cold-temp').textContent = fmt1(cold.temperature);
    document.getElementById('cold-name').textContent = cold.name;
    document.getElementById('cold-county').textContent = COUNTY_NAMES[cold.county] || cold.county;

    document.getElementById('avg-temp').textContent = fmt1(mean);
    document.getElementById('n-stations').textContent = stations.length;
    document.getElementById('spread').textContent = fmt1(max - min).replace('°C', '°');

    /* lists */
    function renderList(el, list) {
      el.innerHTML = list.map(function (s) {
        return '<div class="srow"><span class="nm">' + s.name +
          '<small>' + (COUNTY_NAMES[s.county] || '') + '</small></span>' +
          '<span class="tv" style="color:' + tempColor(s.temperature) + '">' + fmt1(s.temperature) + '</span></div>';
      }).join('');
    }
    renderList(document.getElementById('warm-list'), sorted.slice(0, 8));
    renderList(document.getElementById('cold-list'), sorted.slice(-8).reverse());

    /* histogram: 2° buckets */
    var lo = Math.floor(min / 2) * 2, hi = Math.ceil(max / 2) * 2;
    var labels = [], counts = [];
    for (var b = lo; b < hi; b += 2) {
      labels.push(b + '–' + (b + 2) + '°');
      counts.push(temps.filter(function (t) { return t >= b && t < b + 2; }).length);
    }
    new Chart(document.getElementById('hist'), {
      type: 'bar',
      data: {
        labels: labels,
        datasets: [{
          data: counts,
          backgroundColor: labels.map(function (_, i) { return tempColor(lo + i * 2 + 1) + 'cc'; }),
          borderRadius: 3,
          barPercentage: .85
        }]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { display: false },
          tooltip: { backgroundColor: '#0f1726', borderColor: '#2a3f5f', borderWidth: 1, padding: 10,
                     callbacks: { label: function (c) { return ' ' + c.parsed.y + ' stationer'; } } } },
        scales: {
          x: { grid: { display: false }, ticks: { maxRotation: 0, autoSkip: true } },
          y: { grid: { color: 'rgba(27,41,64,.35)' }, ticks: { precision: 0 } }
        }
      }
    });
  }).catch(function (err) {
    document.getElementById('warm-name').textContent = 'Kunde inte nå API:et';
    console.error('API load failed:', err);
  });
})();
