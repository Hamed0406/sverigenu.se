'use strict';

var COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland',
  k:'Blekinge', m:'Skåne', n:'Halland', o:'Västra Götaland',
  s:'Värmland', t:'Örebro', u:'Västmanland', w:'Dalarna',
  x:'Gävleborg', y:'Västernorrland', z:'Jämtland',
  ac:'Västerbotten', bd:'Norrbotten'
};

var COUNTY_CODES = ['ab','c','d','e','f','g','h','i','k','m','n','o','s','t','u','w','x','y','z','ac','bd'];

var WIND_COLORS = [
  '#56e0a0','#3eb8f5','#f5c842','#e05ae0','#ff9444',
  '#ff6b6b','#a78bfa','#34d399','#fb923c','#60a5fa',
  '#e879f9','#f472b6','#22d3ee','#84cc16','#c084fc',
  '#fbbf24','#4ade80','#f87171','#38bdf8','#a3e635','#e2e8f0'
];

var SOL_COLORS = [
  '#f5c842','#f59e0b','#fbbf24','#d97706','#f97316',
  '#fb923c','#fde047','#facc15','#eab308','#ca8a04',
  '#a16207','#ef4444','#f87171','#fca5a5','#fdba74',
  '#fed7aa','#fef08a','#fef9c3','#fefce8','#f5f5f0','#e2e8f0'
];

// ── Shared chart defaults ────────────────────────────────────────────
var CHART_FONT = { family: "'DM Mono', monospace", size: 11 };
var GRID_COLOR = 'rgba(43,63,95,.4)';
var TICK_COLOR = '#6b82a0';

Chart.defaults.color = TICK_COLOR;
Chart.defaults.font  = CHART_FONT;

function makeScales(yLabel) {
  return {
    x: {
      grid: { color: GRID_COLOR },
      ticks: { color: TICK_COLOR }
    },
    y: {
      grid: { color: GRID_COLOR },
      ticks: { color: TICK_COLOR },
      title: { display: !!yLabel, text: yLabel || '', color: TICK_COLOR }
    }
  };
}

// ── Elpris ───────────────────────────────────────────────────────────
var elprisChart = null;
var elprisData  = {};
var currentZone = 'SE1';

function initElpris() {
  var tabs = document.querySelectorAll('.zone-tab');
  tabs.forEach(function(tab) {
    tab.addEventListener('click', function() {
      tabs.forEach(function(t){ t.classList.remove('active'); });
      tab.classList.add('active');
      currentZone = tab.dataset.zone;
      renderElpris();
    });
  });
  fetch('/api/v1/elpris')
    .then(function(r){ return r.json(); })
    .then(function(d){
      var items = d.data || [];
      ['SE1','SE2','SE3','SE4'].forEach(function(zone){
        elprisData[zone] = items.filter(function(x){ return x.area === zone; })
          .sort(function(a,b){ return a.time_start < b.time_start ? -1 : 1; });
      });
      document.getElementById('elpris-loading').style.display = 'none';
      document.getElementById('elpris-chart-wrap').style.display = '';
      renderElpris();
      // Hero KPI: SE3 current
      var se3 = elprisData['SE3'];
      if (se3 && se3.length) {
        var latest = se3[se3.length - 1];
        var ore = Math.round((latest.sek_per_kwh || 0) * 100);
        document.getElementById('kpi-price').innerHTML = ore + '<span class="kpi-unit">öre/kWh</span>';
      }
    })
    .catch(function(){ document.getElementById('elpris-loading').textContent = 'Kunde inte ladda elprisdata.'; });
}

function renderElpris() {
  var rows = elprisData[currentZone] || [];
  var labels = rows.map(function(r){ return r.time_start ? r.time_start.substring(11,16) : ''; });
  var vals   = rows.map(function(r){ return Math.round((r.sek_per_kwh || 0) * 100 * 10) / 10; });

  var ctx = document.getElementById('elpris-chart');
  if (elprisChart) { elprisChart.destroy(); }
  elprisChart = new Chart(ctx, {
    type: 'line',
    data: {
      labels: labels,
      datasets: [{
        label: currentZone + ' (öre/kWh)',
        data: vals,
        borderColor: '#ff9444',
        backgroundColor: 'rgba(255,148,68,.08)',
        borderWidth: 2,
        pointRadius: 0,
        tension: .3,
        fill: true
      }]
    },
    options: {
      animation: false,
      responsive: true,
      plugins: { legend: { display: false } },
      scales: makeScales('öre/kWh')
    }
  });
}

// ── Energimix ────────────────────────────────────────────────────────
var MIX_COLORS = {
  vattenkraft: '#4ea8f5',
  vindkraft:   '#56e0a0',
  solkraft:    '#f5c842',
  kärnkraft:   '#e05ae0'
};
var MIX_LABELS = {
  vattenkraft: 'Vattenkraft',
  vindkraft:   'Vindkraft',
  solkraft:    'Solkraft',
  kärnkraft:   'Kärnkraft'
};

function initEnergiMix() {
  fetch('/api/v1/energimix?from=1990')
    .then(function(r){ return r.json(); })
    .then(function(d){
      var items = d.data || [];
      if (!items.length) {
        document.getElementById('mix-loading').textContent = 'Ingen energimixdata hittades.';
        return;
      }

      var years = [];
      var bySource = {};
      items.forEach(function(row){
        if (years.indexOf(row.year) === -1) years.push(row.year);
        if (!bySource[row.source]) bySource[row.source] = {};
        bySource[row.source][row.year] = row.twh;
      });
      years.sort(function(a,b){ return a - b; });

      var datasets = Object.keys(MIX_COLORS).map(function(src){
        return {
          label: MIX_LABELS[src],
          data: years.map(function(yr){ return bySource[src] ? (bySource[src][yr] != null ? bySource[src][yr] : null) : null; }),
          borderColor: MIX_COLORS[src],
          backgroundColor: MIX_COLORS[src] + '18',
          borderWidth: 2,
          pointRadius: 0,
          tension: .3,
          fill: false,
          spanGaps: true
        };
      });

      document.getElementById('mix-loading').style.display = 'none';
      document.getElementById('mix-chart-wrap').style.display = '';

      // Hero KPIs from latest year
      var latestYear = years[years.length - 1];
      var prevYear   = years[years.length - 2];
      function getVal(src, yr) {
        return bySource[src] && bySource[src][yr] != null ? bySource[src][yr] : (bySource[src] && bySource[src][prevYear] != null ? bySource[src][prevYear] : null);
      }
      var windTWh    = getVal('vindkraft', latestYear);
      var solarTWh   = getVal('solkraft', latestYear);
      var hydroTWh   = getVal('vattenkraft', latestYear);
      var nuclearTWh = getVal('kärnkraft', latestYear);
      if (windTWh    != null) document.getElementById('kpi-wind').innerHTML    = windTWh.toFixed(1)    + '<span class="kpi-unit">TWh</span>';
      if (solarTWh   != null) document.getElementById('kpi-solar').innerHTML   = solarTWh.toFixed(1)   + '<span class="kpi-unit">TWh</span>';
      if (hydroTWh   != null) document.getElementById('kpi-hydro').innerHTML   = hydroTWh.toFixed(1)   + '<span class="kpi-unit">TWh</span>';
      if (nuclearTWh != null) document.getElementById('kpi-nuclear').innerHTML = nuclearTWh.toFixed(1) + '<span class="kpi-unit">TWh</span>';

      new Chart(document.getElementById('mix-chart'), {
        type: 'line',
        data: { labels: years, datasets: datasets },
        options: {
          responsive: true,
          animation: false,
          plugins: {
            legend: {
              display: true,
              labels: { color: '#a0b4cc', boxWidth: 14, padding: 14, font: CHART_FONT }
            }
          },
          scales: makeScales('TWh')
        }
      });
    })
    .catch(function(){ document.getElementById('mix-loading').textContent = 'Kunde inte ladda energimixdata.'; });
}

// ── County rankings ──────────────────────────────────────────────────
function buildRanking(items, metric, unit, containerId, loadingId, colorClass, yearSel, allData) {
  var years = [];
  items.forEach(function(row){
    if (row.metric === metric && years.indexOf(row.year) === -1) years.push(row.year);
  });
  years.sort(function(a,b){ return b - a; });

  var sel = document.getElementById(yearSel);
  years.forEach(function(yr){
    var opt = document.createElement('option');
    opt.value = yr; opt.textContent = yr; sel.appendChild(opt);
  });

  function render(yr) {
    var byCounty = {};
    items.forEach(function(row){
      if (row.metric === metric && row.year === yr) byCounty[row.county] = row.value;
    });
    var ranked = Object.keys(byCounty)
      .map(function(c){ return { code:c, val:byCounty[c] }; })
      .sort(function(a,b){ return b.val - a.val; });

    if (!ranked.length) { document.getElementById(loadingId).textContent = 'Ingen data för ' + yr; return; }

    var maxVal = ranked[0].val;
    var container = document.getElementById(containerId);
    container.innerHTML = '';
    ranked.forEach(function(row, i){
      var div = document.createElement('div');
      div.className = 'rank-item';
      var pct = maxVal > 0 ? (row.val / maxVal * 100) : 0;
      div.innerHTML = '<span class="rank-num">' + (i+1) + '</span>' +
        '<span class="rank-name">' + (COUNTY_NAMES[row.code] || row.code) + '</span>' +
        '<div class="rank-bar-wrap"><div class="rank-bar ' + colorClass + '" style="width:' + pct.toFixed(1) + '%"></div></div>' +
        '<span class="rank-val">' + (row.val >= 1000 ? (row.val/1000).toFixed(1)+'K' : row.val.toFixed(1)) + ' ' + unit + '</span>';
      container.appendChild(div);
    });
    document.getElementById(loadingId).style.display = 'none';
    container.style.display = '';
  }

  sel.addEventListener('change', function(){ render(parseInt(sel.value)); });
  if (years.length) render(years[0]);
}

// ── County trend charts ──────────────────────────────────────────────
var windTrendChart = null;
var solTrendChart  = null;
var selectedWindCounties = ['bd','z','y'];
var selectedSolCounties  = ['o','m','ab'];

function buildTrendChart(allItems, metric, unit, chartId, picksId, selectedArr, colorArr, onUpdate) {
  var years = [];
  allItems.forEach(function(row){
    if (row.metric === metric && years.indexOf(row.year) === -1) years.push(row.year);
  });
  years.sort(function(a,b){ return a - b; });

  var byCounty = {};
  allItems.forEach(function(row){
    if (row.metric !== metric) return;
    if (!byCounty[row.county]) byCounty[row.county] = {};
    byCounty[row.county][row.year] = row.value;
  });

  function buildDatasets() {
    return selectedArr.map(function(code, i){
      return {
        label: COUNTY_NAMES[code] || code,
        data: years.map(function(yr){ return byCounty[code] ? (byCounty[code][yr] != null ? byCounty[code][yr] : null) : null; }),
        borderColor: colorArr[i % colorArr.length],
        backgroundColor: colorArr[i % colorArr.length] + '22',
        borderWidth: 2,
        pointRadius: 0,
        tension: .3,
        spanGaps: true
      };
    });
  }

  function buildPicks() {
    var wrap = document.getElementById(picksId);
    wrap.innerHTML = '';
    COUNTY_CODES.forEach(function(code, i){
      var btn = document.createElement('button');
      var active = selectedArr.indexOf(code) !== -1;
      btn.textContent = COUNTY_NAMES[code] || code;
      btn.style.cssText = 'font-size:.72rem;padding:.25rem .55rem;border-radius:5px;border:1px solid ' +
        (active ? colorArr[i % colorArr.length] : 'var(--border)') +
        ';background:' + (active ? colorArr[i % colorArr.length] + '22' : 'transparent') +
        ';color:' + (active ? colorArr[i % colorArr.length] : 'var(--muted)') +
        ';cursor:pointer;transition:all .15s;';
      btn.addEventListener('click', function(){
        var idx = selectedArr.indexOf(code);
        if (idx !== -1) {
          selectedArr.splice(idx, 1);
        } else if (selectedArr.length < 5) {
          selectedArr.push(code);
        }
        buildPicks();
        onUpdate();
      });
      wrap.appendChild(btn);
    });
  }

  var chart = new Chart(document.getElementById(chartId), {
    type: 'line',
    data: { labels: years, datasets: buildDatasets() },
    options: {
      responsive: true,
      animation: false,
      plugins: { legend: { display: false } },
      scales: makeScales(unit)
    }
  });

  buildPicks();

  return {
    update: function() {
      chart.data.datasets = buildDatasets();
      chart.update('none');
    }
  };
}

// ── Fetch and wire county data ───────────────────────────────────────
function initCountyData() {
  var windLoaded = false;
  var solLoaded  = false;
  var windItems  = [];
  var solItems   = [];

  fetch('/api/v1/vindkraft')
    .then(function(r){ return r.json(); })
    .then(function(d){
      windItems = (d.data || []).filter(function(r){ return r.metric === 'produktion_gwh' || r.metric === 'installerad_effekt_mw'; });

      buildRanking(windItems, 'installerad_effekt_mw', 'MW',
        'wind-ranking', 'wind-ranking-loading', 'wind', 'wind-year-sel', windItems);

      var windTrendCtrl = buildTrendChart(windItems, 'produktion_gwh', 'GWh',
        'wind-trend-chart', 'trend-county-picks', selectedWindCounties, WIND_COLORS,
        function(){ windTrendCtrl.update(); });
      windTrendChart = windTrendCtrl;
    })
    .catch(function(){
      document.getElementById('wind-ranking-loading').textContent = 'Kunde inte ladda vindkraftdata.';
    });

  fetch('/api/v1/solceller')
    .then(function(r){ return r.json(); })
    .then(function(d){
      solItems = (d.data || []);

      buildRanking(solItems, 'installerad_effekt_mw', 'MW',
        'sol-ranking', 'sol-ranking-loading', 'solar', 'sol-year-sel', solItems);

      var solTrendCtrl = buildTrendChart(solItems, 'installerad_effekt_mw', 'MW',
        'sol-trend-chart', 'sol-trend-county-picks', selectedSolCounties, SOL_COLORS,
        function(){ solTrendCtrl.update(); });
      solTrendChart = solTrendCtrl;
    })
    .catch(function(){
      document.getElementById('sol-ranking-loading').textContent = 'Kunde inte ladda solcellsdata.';
    });
}

// ── Bootstrap ────────────────────────────────────────────────────────
initElpris();
initEnergiMix();
initCountyData();
