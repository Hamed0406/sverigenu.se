(function () {
  'use strict';

  var COUNTY_COLORS = {
    ab: '#4fa3e8', o: '#2dd4a0', m: '#7c6cff', c: '#79c0ff',
    e: '#d29922', f: '#56d364', n: '#ff7b72', bd: '#38b6ff',
    x: '#bc8cff', y: '#f78166', z: '#3fb950', w: '#e3b341',
    d: '#6cb6ff', g: '#57ab5a', h: '#e0823d', i: '#b083f0',
    k: '#ec8e7b', s: '#54aeff', t: '#6fdd8b', u: '#daaa3f', ac: '#8ddb8c'
  };
  var DEFAULT_ON = ['ab', 'o', 'm', 'bd'];

  var fmt  = new Intl.NumberFormat('sv-SE', { maximumFractionDigits: 1 });
  var fmtI = new Intl.NumberFormat('sv-SE');

  /* Chart.js dark defaults */
  Chart.defaults.color = '#6b82a0';
  Chart.defaults.borderColor = 'rgba(27,41,64,.6)';
  Chart.defaults.font.family = "'DM Mono', monospace";
  Chart.defaults.font.size = 10;

  var byCountyMean   = {};  // code -> [{year, value}]
  var byCountyMedian = {};
  var names = {};
  var selected = DEFAULT_ON.slice();
  var chart = null;

  function countUp(el, target, dur, formatter) {
    var start = null;
    function frame(ts) {
      if (!start) start = ts;
      var t = Math.min((ts - start) / dur, 1);
      var eased = 1 - Math.pow(1 - t, 3);
      el.textContent = formatter(target * eased);
      if (t < 1) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }

  function fmtTkr(v) { return fmt.format(v) + ' tkr'; }

  function buildChart() {
    var ctx = document.getElementById('ts-chart');
    var datasets = [];
    selected.forEach(function (code) {
      var mean   = byCountyMean[code]   || [];
      var median = byCountyMedian[code] || [];
      var color  = COUNTY_COLORS[code] || '#4fa3e8';
      var label  = names[code] || code.toUpperCase();
      datasets.push({
        label: label + ' (snitt)',
        data: mean.map(function (r) { return { x: r.year, y: r.value }; }),
        borderColor: color,
        backgroundColor: 'transparent',
        borderWidth: 2,
        pointRadius: 0, pointHoverRadius: 4,
        pointHoverBackgroundColor: color,
        tension: .25
      });
      datasets.push({
        label: label + ' (median)',
        data: median.map(function (r) { return { x: r.year, y: r.value }; }),
        borderColor: color,
        backgroundColor: 'transparent',
        borderWidth: 1.5,
        borderDash: [4, 3],
        pointRadius: 0, pointHoverRadius: 3,
        pointHoverBackgroundColor: color,
        tension: .25
      });
    });

    if (chart) { chart.destroy(); }
    chart = new Chart(ctx, {
      type: 'line',
      data: { datasets: datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'nearest', axis: 'x', intersect: false },
        plugins: {
          legend: {
            labels: { usePointStyle: true, pointStyle: 'line', boxWidth: 24, padding: 16,
                      font: { family: "'DM Sans', sans-serif", size: 11 } }
          },
          tooltip: {
            backgroundColor: '#0f1726',
            borderColor: '#2a3f5f',
            borderWidth: 1,
            titleFont: { family: "'Fraunces', serif", size: 13 },
            bodyFont: { family: "'DM Mono', monospace", size: 11 },
            padding: 12,
            callbacks: {
              label: function (item) {
                return ' ' + item.dataset.label + ': ' + fmt.format(item.parsed.y) + ' tkr';
              }
            }
          }
        },
        scales: {
          x: { type: 'linear', min: 1991, max: 2024,
               ticks: { stepSize: 6, callback: function (v) { return v; } },
               grid: { color: 'rgba(27,41,64,.35)' } },
          y: { ticks: { callback: function (v) { return v + ' tkr'; } },
               grid: { color: 'rgba(27,41,64,.35)' } }
        }
      }
    });
  }

  function buildPicker() {
    var picker = document.getElementById('picker');
    picker.innerHTML = '';
    Object.keys(names).forEach(function (code) {
      var b = document.createElement('button');
      b.className = 'pick' + (selected.indexOf(code) !== -1 ? ' on' : '');
      b.textContent = names[code];
      b.style.setProperty('--pc', COUNTY_COLORS[code] || '#4fa3e8');
      b.addEventListener('click', function () {
        var i = selected.indexOf(code);
        if (i !== -1) {
          selected.splice(i, 1);
        } else {
          if (selected.length >= 6) selected.shift();
          selected.push(code);
        }
        buildPicker();
        buildChart();
      });
      picker.appendChild(b);
    });
  }

  function buildIneq(latestMeans, latestMedians) {
    /* sort by mean descending */
    var rows = latestMeans.slice().sort(function (a, b) { return b.value - a.value; });
    var maxMean = rows[0].value;

    /* build median lookup */
    var medianMap = {};
    latestMedians.forEach(function (r) { medianMap[r.county] = r.value; });

    var grid = document.getElementById('ineq-grid');
    rows.forEach(function (r) {
      var median = medianMap[r.county] || 0;
      var meanW   = (r.value  / maxMean * 100).toFixed(1);
      var medianW = (median   / maxMean * 100).toFixed(1);
      var row = document.createElement('div');
      row.className = 'ineq-row';
      row.innerHTML =
        '<div class="ineq-name">' + (names[r.county] || r.county) + '</div>' +
        '<div class="ineq-track">' +
          '<div class="ineq-mean"   data-w="' + meanW   + '"></div>' +
          '<div class="ineq-median" data-w="' + medianW + '"></div>' +
        '</div>' +
        '<div class="ineq-val">' + fmt.format(r.value) + ' <span style="color:var(--dim)">/ ' + fmt.format(median) + '</span> tkr</div>';
      grid.appendChild(row);
    });

    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (!en.isIntersecting) return;
        var kids = Array.prototype.slice.call(grid.children);
        kids.forEach(function (row, i) {
          setTimeout(function () {
            row.classList.add('in');
            row.querySelector('.ineq-mean').style.width   = row.querySelector('.ineq-mean').dataset.w   + '%';
            row.querySelector('.ineq-median').style.width = row.querySelector('.ineq-median').dataset.w + '%';
          }, i * 40);
        });
        io.disconnect();
      });
    }, { threshold: .15 });
    io.observe(grid);
  }

  function buildUnempTables(rows, countyNames) {
    rows.sort(function (a, b) { return b.value - a.value; });
    var high = rows.slice(0, 10);
    var low  = rows.slice(-10).reverse();

    function render(tbodyId, list) {
      var tbody = document.getElementById(tbodyId);
      list.forEach(function (m, i) {
        var tr = document.createElement('tr');
        tr.innerHTML =
          '<td class="rank">' + (i + 1) + '</td>' +
          '<td><span class="mname">' + m.name + '</span> <span class="mcounty">' + (countyNames[m.county] || m.county) + '</span></td>' +
          '<td class="mval">' + m.value.toFixed(1).replace('.', ',') + '&thinsp;%</td>';
        tbody.appendChild(tr);
      });
    }
    render('high-unemp', high);
    render('low-unemp', low);
  }

  /* ── Load everything ─────────────────────────────── */
  Promise.all([
    fetch('/api/v1/lan/summary').then(function (r) { return r.json(); }),
    fetch('/api/v1/inkomst').then(function (r) { return r.json(); }),
    fetch('/api/v1/kolada?kpi=N03920').then(function (r) { return r.json(); }),
    fetch('/api/v1/kolada?kpi=N00989').then(function (r) { return r.json(); }),
    fetch('/api/v1/kolada?kpi=N00990').then(function (r) { return r.json(); })
  ]).then(function (res) {
    var summary    = res[0].data || [];
    var inkomst    = res[1].data || [];
    var unempRaw   = res[2].data || [];
    var debtPctRaw = res[3].data || [];
    var debtMedRaw = res[4].data || [];

    summary.forEach(function (r) { names[r.code] = r.name; });

    /* organise inkomst by county+metric */
    inkomst.forEach(function (r) {
      if (r.metric === 'mean') {
        (byCountyMean[r.county]   = byCountyMean[r.county]   || []).push({ year: r.year, value: r.value });
      } else {
        (byCountyMedian[r.county] = byCountyMedian[r.county] || []).push({ year: r.year, value: r.value });
      }
    });
    Object.keys(byCountyMean).forEach(function (c) {
      byCountyMean[c].sort(function (a, b) { return a.year - b.year; });
    });
    Object.keys(byCountyMedian).forEach(function (c) {
      byCountyMedian[c].sort(function (a, b) { return a.year - b.year; });
    });

    /* hero stats — latest year */
    var latestYear = 0;
    inkomst.forEach(function (r) { if (r.year > latestYear) latestYear = r.year; });

    var latestMeans   = inkomst.filter(function (r) { return r.year === latestYear && r.metric === 'mean'; });
    var latestMedians = inkomst.filter(function (r) { return r.year === latestYear && r.metric === 'median'; });

    var sumMean = 0, sumMedian = 0;
    latestMeans.forEach(function (r)   { sumMean   += r.value; });
    latestMedians.forEach(function (r) { sumMedian += r.value; });
    var natMean   = sumMean   / (latestMeans.length   || 1);
    var natMedian = sumMedian / (latestMedians.length || 1);

    countUp(document.getElementById('nat-mean'),   natMean,   1400, function (v) { return fmt.format(v) + ' tkr'; });
    countUp(document.getElementById('nat-median'), natMedian, 1400, function (v) { return fmt.format(v) + ' tkr'; });
    document.getElementById('nat-year').textContent   = 'snitt ' + latestYear + ' · 21 län';
    document.getElementById('median-sub').textContent = 'median ' + latestYear + ' · 21 län';

    latestMeans.sort(function (a, b) { return b.value - a.value; });
    if (latestMeans.length) {
      var top = latestMeans[0];
      var bot = latestMeans[latestMeans.length - 1];
      document.getElementById('top-lan').textContent = names[top.county] || top.county;
      document.getElementById('top-val').textContent = fmt.format(top.value) + ' tkr';
      document.getElementById('bot-lan').textContent = names[bot.county] || bot.county;
      document.getElementById('bot-val').textContent = fmt.format(bot.value) + ' tkr';
    }

    buildPicker();
    buildChart();
    buildIneq(latestMeans, latestMedians);

    /* unemployment tables */
    var unempLatest = {};
    unempRaw.forEach(function (r) {
      var key = r.municipality;
      if (!unempLatest[key] || r.period > unempLatest[key].period) {
        unempLatest[key] = r;
      }
    });
    var unempRows = Object.values(unempLatest).filter(function (r) { return r.value !== null; });
    buildUnempTables(unempRows, names);

    /* Kronofogden — sort by value desc, top 10 */
    function buildDebtTable(tbodyId, rows, formatFn) {
      var sorted = rows.slice().sort(function (a, b) { return b.value - a.value; });
      var tbody = document.getElementById(tbodyId);
      if (!tbody) return;
      sorted.slice(0, 10).forEach(function (r) {
        var tr = document.createElement('tr');
        tr.innerHTML = '<td>' + (r.name || r.municipality) + '</td>' +
                       '<td style="font-family:\'DM Mono\',monospace;text-align:right;color:var(--gold)">' + formatFn(r.value) + '</td>';
        tbody.appendChild(tr);
      });
    }
    if (debtPctRaw.length) buildDebtTable('high-debt', debtPctRaw, function (v) { return v.toFixed(1).replace('.', ',') + ' %'; });
    if (debtMedRaw.length) buildDebtTable('high-debt-median', debtMedRaw, function (v) { return Math.round(v).toLocaleString('sv-SE') + ' kr'; });

  }).catch(function (err) {
    document.getElementById('chart-empty').hidden = false;
    console.error('API load failed:', err);
  });
})();
