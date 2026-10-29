// /samband — cross-source correlation explorer.
// One scatter, two axis pickers, Pearson r, trend line, outliers, county highlight.
(function () {
  'use strict';

  var BASE_PT = '#5b8bc4';   // all municipalities (validated vs dark surface)
  var HL_PT   = '#c98330';   // highlighted county (CVD-separated from base)
  var TREND   = '#e8a13c';
  var GRID    = 'rgba(43,63,95,.35)';
  var TICK    = '#6b82a0';

  var METRICS = {
    skattesats: {
      label: 'Total skattesats', unit: '%', src: 'Skatteverket 2025',
      desc: 'Hur många procent av lönen som går till kommun och region innan kyrkoavgift. Bestäms av varje kommun — därför skiljer det nästan 7 procentenheter mellan billigaste och dyraste kommunen.',
      high: 'hög skatt', low: 'låg skatt',
      fmt: function (v) { return v.toFixed(2) + ' %'; }
    },
    arbetsloshet: {
      label: 'Arbetslöshet 18–64 år', unit: '%', src: 'Kolada/AF senaste år',
      desc: 'Andel av kommunens invånare i arbetsför ålder som är öppet arbetslösa eller i program med aktivitetsstöd. Ett direkt mått på hur den lokala arbetsmarknaden mår.',
      high: 'många utan jobb', low: 'få utan jobb',
      fmt: function (v) { return v.toFixed(1) + ' %'; }
    },
    skuldsatta: {
      label: 'Skuldsatta hos Kronofogden', unit: '%', src: 'Kolada/Kronofogden senaste år',
      desc: 'Andel vuxna med obetalda skulder som gått så långt att Kronofogden driver in dem. Fångar hushåll där ekonomin redan har havererat — inte vanliga bolån eller krediter.',
      high: 'många med indrivna skulder', low: 'få med indrivna skulder',
      fmt: function (v) { return v.toFixed(1) + ' %'; }
    },
    medianskuld: {
      label: 'Medianskuld hos Kronofogden', unit: 'kr', src: 'Kolada/Kronofogden senaste år',
      desc: 'Den typiska skulden (mittenvärdet) bland de som finns hos Kronofogden i kommunen. Visar hur djupt skuldproblemen sitter, inte hur många som har dem.',
      high: 'stora skulder per person', low: 'mindre skulder per person',
      fmt: function (v) { return Math.round(v).toLocaleString('sv-SE') + ' kr'; }
    },
    meritvarde: {
      label: 'Meritvärde åk 9', unit: 'poäng', src: 'Kolada/Skolverket senaste år',
      desc: 'Genomsnittligt slutbetyg för niondeklassare (max 340 poäng — summan av de 17 bästa ämnesbetygen). Standardmåttet på hur skolan lyckas i kommunen.',
      high: 'höga betyg', low: 'låga betyg',
      fmt: function (v) { return v.toFixed(0) + ' p'; }
    },
    brott: {
      label: 'Anmälda brott per 100 000 inv.', unit: 'per 100k', src: 'BRÅ senaste år',
      desc: 'Alla polisanmälda brott under året, omräknat per 100 000 invånare så att stora och små kommuner kan jämföras. Anmälda brott — inte uppklarade eller dömda.',
      high: 'många anmälda brott', low: 'få anmälda brott',
      fmt: function (v) { return Math.round(v).toLocaleString('sv-SE'); }
    },
    median_skatt: {
      label: 'Median direkt skatt på arbete', unit: 'kr/år', src: 'Skatteverket 2023',
      desc: 'Vad den typiska invånaren faktiskt betalade i inkomstskatt på arbete under året, i kronor. Skiljer sig från skattesatsen: en rik kommun med låg skattesats kan ändå betala mest per person eftersom lönerna är högre.',
      high: 'betalar mycket i kronor', low: 'betalar lite i kronor',
      fmt: function (v) { return Math.round(v).toLocaleString('sv-SE') + ' kr'; }
    },
    bistand: {
      label: 'Ekonomiskt bistånd', unit: 'kr/inv', src: 'Kolada 2023',
      desc: 'Utbetalt försörjningsstöd ("socialbidrag") under året, utslaget per invånare i kommunen. Ett mått på hur många hushåll som inte klarar sig på egen inkomst.',
      high: 'mycket bistånd betalas ut', low: 'lite bistånd betalas ut',
      fmt: function (v) { return Math.round(v).toLocaleString('sv-SE') + ' kr'; }
    },
    befolkning: {
      label: 'Folkmängd', unit: 'inv.', src: 'Kolada/SCB senaste år',
      desc: 'Antal invånare i kommunen. Används också som punktstorlek i diagrammet — stora bubblor är stora kommuner.',
      high: 'stor kommun', low: 'liten kommun',
      fmt: function (v) { return Math.round(v).toLocaleString('sv-SE'); }
    },
    fornlamningar_1000: {
      label: 'Fornlämningar per 1 000 inv.', unit: 'st/1000 inv', src: 'Riksantikvarieämbetet, löpande',
      desc: 'Registrerade fornlämningar (gravfält, runstenar, boplatser med mera) i förhållande till folkmängden. Höga värden betyder gles landsbygd med lång bosättningshistoria — Gotland toppar stort.',
      high: 'mycket kulturarv per invånare', low: 'lite kulturarv per invånare',
      fmt: function (v) { return v.toFixed(1); }
    }
  };

  var COUNTY_NAMES = {
    ab: 'Stockholm', c: 'Uppsala', d: 'Södermanland', e: 'Östergötland', f: 'Jönköping',
    g: 'Kronoberg', h: 'Kalmar', i: 'Gotland', k: 'Blekinge', m: 'Skåne', n: 'Halland',
    o: 'Västra Götaland', s: 'Värmland', t: 'Örebro', u: 'Västmanland', w: 'Dalarna',
    x: 'Gävleborg', y: 'Västernorrland', z: 'Jämtland', ac: 'Västerbotten', bd: 'Norrbotten'
  };

  var rows = [];
  var chart = null;
  var kapChart = null;

  function titleCase(s) {
    return s.toLowerCase().replace(/(^|[\s-])([a-zåäöé])/g, function (m) { return m.toUpperCase(); });
  }

  function pearson(pairs) {
    var n = pairs.length;
    if (n < 3) return NaN;
    var sx = 0, sy = 0, sxx = 0, syy = 0, sxy = 0;
    pairs.forEach(function (p) {
      sx += p[0]; sy += p[1]; sxx += p[0] * p[0]; syy += p[1] * p[1]; sxy += p[0] * p[1];
    });
    var cov = sxy - sx * sy / n;
    var vx = sxx - sx * sx / n;
    var vy = syy - sy * sy / n;
    if (vx <= 0 || vy <= 0) return NaN;
    return cov / Math.sqrt(vx * vy);
  }

  function regression(pairs) {
    var n = pairs.length, sx = 0, sy = 0, sxx = 0, sxy = 0;
    pairs.forEach(function (p) { sx += p[0]; sy += p[1]; sxx += p[0] * p[0]; sxy += p[0] * p[1]; });
    var b = (sxy - sx * sy / n) / (sxx - sx * sx / n);
    var a = sy / n - b * sx / n;
    return { a: a, b: b };
  }

  function strength(r) {
    var a = Math.abs(r);
    if (a < 0.2) return 'försumbart';
    if (a < 0.4) return 'svagt';
    if (a < 0.6) return 'måttligt';
    if (a < 0.8) return 'starkt';
    return 'mycket starkt';
  }

  function fmtR(r) {
    return (r >= 0 ? '+' : '−') + Math.abs(r).toFixed(2);
  }

  function radius(pop) {
    if (!pop) return 4;
    return Math.max(3, Math.min(15, Math.sqrt(pop) / 65));
  }

  function currentPairs(xKey, yKey) {
    return rows.filter(function (r) {
      return r[xKey] != null && r[yKey] != null;
    });
  }

  function render() {
    var xKey = document.getElementById('ax-x').value;
    var yKey = document.getElementById('ax-y').value;
    var hl = document.getElementById('hl-county').value;
    var mx = METRICS[xKey], my = METRICS[yKey];

    var data = currentPairs(xKey, yKey);
    var pairs = data.map(function (r) { return [r[xKey], r[yKey]]; });
    var r = pearson(pairs);
    var reg = regression(pairs);

    document.getElementById('corr-val').textContent = isNaN(r) ? '–' : fmtR(r);
    var plain = '';
    if (!isNaN(r)) {
      if (Math.abs(r) < 0.2) {
        plain = 'Inget tydligt mönster: kommuner med ' + mx.high + ' kan ha både ' + my.high + ' och ' + my.low + '.';
      } else if (r > 0) {
        plain = 'Enkelt uttryckt: kommuner med ' + mx.high + ' har oftast också ' + my.high + '.';
      } else {
        plain = 'Enkelt uttryckt: kommuner med ' + mx.high + ' har oftast ' + my.low + '.';
      }
    }
    document.getElementById('corr-text').innerHTML = isNaN(r) ? '' :
      'Sambandet mellan <strong>' + mx.label.toLowerCase() + '</strong> och <strong>' + my.label.toLowerCase() +
      '</strong> är <strong>' + strength(r) + '</strong> (' + data.length + ' kommuner). ' + plain;

    document.getElementById('axis-desc').innerHTML =
      '<div class="ad"><b>X-axel · ' + mx.label + '</b>' + mx.desc + '</div>' +
      '<div class="ad"><b>Y-axel · ' + my.label + '</b>' + my.desc + '</div>';

    var base = [], hilite = [];
    data.forEach(function (row) {
      var pt = { x: row[xKey], y: row[yKey], r: radius(row.befolkning), name: row.municipality, county: row.county };
      (hl !== 'all' && row.county === hl ? hilite : base).push(pt);
    });

    var xs = pairs.map(function (p) { return p[0]; });
    var xmin = Math.min.apply(null, xs), xmax = Math.max.apply(null, xs);
    var trendData = [
      { x: xmin, y: reg.a + reg.b * xmin },
      { x: xmax, y: reg.a + reg.b * xmax }
    ];

    var tooltipCb = {
      label: function (ctx) {
        var p = ctx.raw;
        return [
          ' ' + titleCase(p.name) + ' (' + (COUNTY_NAMES[p.county] || p.county) + ')',
          ' ' + mx.label + ': ' + mx.fmt(p.x),
          ' ' + my.label + ': ' + my.fmt(p.y)
        ];
      }
    };

    var ctx = document.getElementById('scatter').getContext('2d');
    if (chart) chart.destroy();
    chart = new Chart(ctx, {
      data: {
        datasets: [
          {
            type: 'scatter', label: 'Kommun', data: base, order: 3,
            backgroundColor: BASE_PT + 'a8', borderColor: BASE_PT, borderWidth: 1,
            pointRadius: function (c) { return c.raw ? c.raw.r : 4; },
            pointHoverRadius: function (c) { return (c.raw ? c.raw.r : 4) + 3; }
          },
          {
            type: 'scatter', label: hl !== 'all' ? (COUNTY_NAMES[hl] || hl) : '', data: hilite, order: 1,
            backgroundColor: HL_PT + 'e6', borderColor: '#0f1726', borderWidth: 2,
            pointRadius: function (c) { return (c.raw ? c.raw.r : 4) + 1; },
            pointHoverRadius: function (c) { return (c.raw ? c.raw.r : 4) + 4; }
          },
          {
            type: 'line', label: 'Trendlinje', data: trendData, order: 2,
            borderColor: TREND, borderWidth: 2, borderDash: [6, 5],
            pointRadius: 0, fill: false, tension: 0
          }
        ]
      },
      options: {
        responsive: true, maintainAspectRatio: false, animation: { duration: 250 },
        interaction: { mode: 'nearest', intersect: true },
        plugins: {
          legend: { display: hl !== 'all', labels: { color: TICK, usePointStyle: true, filter: function (item) { return item.text !== 'Kommun' && item.text !== 'Trendlinje' && item.text !== ''; } } },
          tooltip: { callbacks: tooltipCb, displayColors: false }
        },
        scales: {
          x: { title: { display: true, text: mx.label + ' (' + mx.unit + ')', color: TICK, font: { size: 12 } },
               grid: { color: GRID }, ticks: { color: TICK, font: { family: "'DM Mono', monospace", size: 11 } } },
          y: { title: { display: true, text: my.label + ' (' + my.unit + ')', color: TICK, font: { size: 12 } },
               grid: { color: GRID }, ticks: { color: TICK, font: { family: "'DM Mono', monospace", size: 11 } } }
        }
      }
    });

    document.getElementById('scatter-note').textContent =
      'Punktstorlek = folkmängd. Streckad linje = linjär trend. Källor: ' + mx.src + ' · ' + my.src + '.';

    renderOutliers(data, xKey, yKey, reg, my);
    markActiveStory(xKey, yKey);
  }

  function renderOutliers(data, xKey, yKey, reg, my) {
    var withRes = data.map(function (row) {
      return { row: row, res: row[yKey] - (reg.a + reg.b * row[xKey]) };
    }).sort(function (a, b) { return b.res - a.res; });

    var above = withRes.slice(0, 5);
    var below = withRes.slice(-5).reverse();
    document.getElementById('out-above-title').textContent = 'Högre ' + my.label.toLowerCase() + ' än förväntat';
    document.getElementById('out-below-title').textContent = 'Lägre ' + my.label.toLowerCase() + ' än förväntat';

    function fill(el, list) {
      el.innerHTML = '';
      list.forEach(function (o, i) {
        var d = document.createElement('div');
        d.className = 'rank-item';
        d.innerHTML = '<span class="rank-n">' + (i + 1) + '</span>' +
          '<span class="rank-name">' + titleCase(o.row.municipality) + '</span>' +
          '<span class="rank-val">' + my.fmt(o.row[yKey]) + '</span>';
        el.appendChild(d);
      });
    }
    fill(document.getElementById('out-above'), above);
    fill(document.getElementById('out-below'), below);
  }

  function markActiveStory(xKey, yKey) {
    document.querySelectorAll('.story').forEach(function (btn) {
      btn.classList.toggle('active', btn.dataset.x === xKey && btn.dataset.y === yKey);
    });
  }

  function initControls() {
    var selX = document.getElementById('ax-x');
    var selY = document.getElementById('ax-y');
    Object.keys(METRICS).forEach(function (k) {
      selX.appendChild(new Option(METRICS[k].label, k));
      selY.appendChild(new Option(METRICS[k].label, k));
    });
    selX.value = 'arbetsloshet';
    selY.value = 'skuldsatta';

    var selC = document.getElementById('hl-county');
    selC.appendChild(new Option('Inget', 'all'));
    Object.keys(COUNTY_NAMES).sort(function (a, b) {
      return COUNTY_NAMES[a].localeCompare(COUNTY_NAMES[b], 'sv');
    }).forEach(function (c) {
      selC.appendChild(new Option(COUNTY_NAMES[c], c));
    });

    selX.addEventListener('change', render);
    selY.addEventListener('change', render);
    selC.addEventListener('change', render);

    document.querySelectorAll('.story').forEach(function (btn) {
      var pairs = currentPairs(btn.dataset.x, btn.dataset.y).map(function (r) {
        return [r[btn.dataset.x], r[btn.dataset.y]];
      });
      var r = pearson(pairs);
      btn.querySelector('[data-r]').textContent = isNaN(r) ? '–' : 'r ' + fmtR(r);
      btn.addEventListener('click', function () {
        selX.value = btn.dataset.x;
        selY.value = btn.dataset.y;
        render();
        document.getElementById('utforska').scrollIntoView({ behavior: 'smooth', block: 'start' });
      });
    });
  }

  function initMetricGuide() {
    var grid = document.getElementById('metric-grid');
    if (!grid) return;
    Object.keys(METRICS).forEach(function (k) {
      var m = METRICS[k];
      var card = document.createElement('div');
      card.className = 'metric-card';
      card.innerHTML =
        '<h3>' + m.label + '</h3>' +
        '<div class="m-src">' + m.src + ' · enhet: ' + m.unit + '</div>' +
        '<p>' + m.desc + '</p>' +
        '<div class="m-hl"><span>Högt = <b>' + m.high + '</b></span><span>Lågt = <b>' + m.low + '</b></span></div>';
      grid.appendChild(card);
    });
  }

  function initTopR() {
    var keys = Object.keys(METRICS).filter(function (k) { return k !== 'befolkning'; });
    var best = { r: 0, x: null, y: null };
    for (var i = 0; i < keys.length; i++) {
      for (var j = i + 1; j < keys.length; j++) {
        var pairs = currentPairs(keys[i], keys[j]).map(function (r) { return [r[keys[i]], r[keys[j]]]; });
        var r = pearson(pairs);
        if (!isNaN(r) && Math.abs(r) > Math.abs(best.r)) best = { r: r, x: keys[i], y: keys[j] };
      }
    }
    if (best.x) {
      document.getElementById('kpi-topr').textContent = fmtR(best.r);
      document.getElementById('kpi-topr-name').textContent =
        METRICS[best.x].label + ' × ' + METRICS[best.y].label;
    }
  }

  function initKapital(kap) {
    var latest = kap.reduce(function (m, r) { return Math.max(m, r.year); }, 0);
    var data = kap.filter(function (r) {
      return r.year === latest && r.antal > 0 && r.county_name.indexOf('Summering') === -1;
    }).map(function (r) {
      return { name: titleCase(r.county_name), perCapita: r.belopp_sek / r.antal };
    }).sort(function (a, b) { return b.perCapita - a.perCapita; });

    var ctx = document.getElementById('kapital-bar').getContext('2d');
    if (kapChart) kapChart.destroy();
    kapChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels: data.map(function (d) { return d.name; }),
        datasets: [{
          label: 'Överskott av kapital per mottagare (kr)',
          data: data.map(function (d) { return Math.round(d.perCapita); }),
          backgroundColor: BASE_PT + 'cc', borderColor: BASE_PT, borderWidth: 1,
          borderRadius: 4, borderSkipped: 'start', barPercentage: 0.82
        }]
      },
      options: {
        indexAxis: 'y', responsive: true, maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: { displayColors: false, callbacks: {
            label: function (c) { return ' ' + c.parsed.x.toLocaleString('sv-SE') + ' kr per mottagare (' + latest + ')'; }
          } }
        },
        scales: {
          x: { grid: { color: GRID }, ticks: { color: TICK, font: { family: "'DM Mono', monospace", size: 11 },
               callback: function (v) { return (v / 1000) + ' tkr'; } } },
          y: { grid: { display: false }, ticks: { color: TICK, font: { size: 11 } } }
        }
      }
    });
  }

  Promise.all([
    fetch('/api/v1/samband').then(function (r) { return r.json(); }),
    fetch('/api/v1/kapitalinkomst?year=all&term=' + encodeURIComponent('Överskott av kapital')).then(function (r) { return r.json(); })
  ]).then(function (res) {
    rows = res[0].data || [];
    rows.forEach(function (r) {
      r.fornlamningar_1000 = (r.fornlamningar != null && r.befolkning > 0)
        ? r.fornlamningar / r.befolkning * 1000 : null;
    });
    initControls();
    initMetricGuide();
    initTopR();
    render();
    initKapital(res[1].data || []);
  }).catch(function (e) {
    document.getElementById('corr-text').textContent = 'Kunde inte hämta data: ' + e.message;
  });
})();
