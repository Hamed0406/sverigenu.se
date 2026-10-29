(function () {
  'use strict';

  const COUNTY_NAMES = {
    ab: 'Stockholm', c: 'Uppsala', d: 'Södermanland', e: 'Östergötland',
    f: 'Jönköping', g: 'Kronoberg', h: 'Kalmar', i: 'Gotland',
    k: 'Blekinge', m: 'Skåne', n: 'Halland', o: 'Västra Götaland',
    s: 'Värmland', t: 'Örebro', u: 'Västmanland', w: 'Dalarna',
    x: 'Gävleborg', y: 'Västernorrland', z: 'Jämtland',
    ac: 'Västerbotten', bd: 'Norrbotten'
  };

  const PALETTE = [
    '#f5a623','#38b6ff','#3fb950','#e05ae0','#ff7b72',
    '#ffd166','#06d6a0','#a29bfe','#fd79a8','#74b9ff',
    '#00cec9','#ff6b6b','#55efc4','#b2bec3','#fdcb6e',
    '#e17055','#81ecec','#fab1d3','#636e72','#dfe6e9',
    '#2d3436'
  ];

  let allSkatt = [];
  let allInkomst = [];
  let allFoetag = [];
  let allKapital = [];
  let allSPP = [];
  let allNiva = [];
  let trendChart = null;
  let foChart = null;
  let kiBarChart = null;
  let kiTrendChart = null;
  let sppChart = null;
  let nivaChart = null;
  let nivaTrendChart = null;
  let trendSelected = new Set(['ab', 'o', 'm', 'i', 'bd']);

  // ── Fetch ────────────────────────────────────────────────────────────────
  async function init() {
    const [skattRes, inkomstRes, foRes, kiRes, sppRes, nivaRes] = await Promise.all([
      fetch('/api/v1/skattesats?year=all'),
      fetch('/api/v1/inkomst'),
      fetch('/api/v1/foretagsoverskott?year=all'),
      fetch('/api/v1/kapitalinkomst?year=all'),
      fetch('/api/v1/skatt/per-person?year=all'),
      fetch('/api/v1/skatt/inkomstniva?year=all&term=all')
    ]);
    const skattData = await skattRes.json();
    const inkomstData = await inkomstRes.json();
    const foData = await foRes.json();
    const kiData = await kiRes.json();
    const sppData = await sppRes.json();
    const nivaData = await nivaRes.json();

    allSkatt = skattData.data || [];
    allInkomst = inkomstData.data || [];
    allFoetag = foData.data || [];
    allKapital = kiData.data || [];
    allSPP = sppData.data || [];
    allNiva = nivaData.data || [];

    populateYearSelect();
    renderHeroKPIs();
    renderRankings();
    renderCountyBar();
    renderTrendPicker();
    renderTrendChart();
    renderScatter();
    initForetagsoverskott();
    initKapitalinkomst();
    initSkattPerPerson();
    initInkomstniva();
  }

  function getSkattForYear(year) {
    return allSkatt.filter(r => r.year === year);
  }

  // ── Year select ──────────────────────────────────────────────────────────
  function populateYearSelect() {
    const years = [...new Set(allSkatt.map(r => r.year))].sort((a, b) => b - a);
    const sel = document.getElementById('rank-year');
    years.forEach(y => {
      const o = document.createElement('option');
      o.value = y;
      o.textContent = y;
      if (y === 2025) o.selected = true;
      sel.appendChild(o);
    });
    sel.addEventListener('change', renderRankings);
    document.getElementById('rank-sort').addEventListener('change', renderRankings);
  }

  // ── Hero KPIs ────────────────────────────────────────────────────────────
  function renderHeroKPIs() {
    const rows2025 = getSkattForYear(2025);
    if (!rows2025.length) return;
    const vals = rows2025.map(r => r.kommunal_skatt).sort((a, b) => a - b);
    const avg = (vals.reduce((s, v) => s + v, 0) / vals.length).toFixed(2);
    const min = vals[0];
    const max = vals[vals.length - 1];
    const minRow = rows2025.find(r => r.kommunal_skatt === min);
    const maxRow = rows2025.find(r => r.kommunal_skatt === max);

    setText('kpi-avg', avg + '%');
    setText('kpi-min', min + '%');
    setText('kpi-min-name', titleCase(minRow.municipality));
    setText('kpi-max', max + '%');
    setText('kpi-max-name', titleCase(maxRow.municipality));
    setText('kpi-span', (max - min).toFixed(1) + ' pp');
  }

  // ── Rankings ─────────────────────────────────────────────────────────────
  function renderRankings() {
    const year = parseInt(document.getElementById('rank-year').value, 10);
    const sort = document.getElementById('rank-sort').value;
    let rows = getSkattForYear(year);

    const keyFn = sort.startsWith('total') ? r => r.summa_exkl_kyrka : r => r.kommunal_skatt;
    const desc = sort.endsWith('desc');
    rows = [...rows].sort((a, b) => desc ? keyFn(b) - keyFn(a) : keyFn(a) - keyFn(b));

    const vals = rows.map(r => keyFn(r));
    const minV = Math.min(...vals);
    const maxV = Math.max(...vals);
    const pct = v => Math.round((v - minV) / (maxV - minV) * 100);

    const high = rows.slice(0, 20);
    const low = rows.slice(-20).reverse();

    renderList('rank-high', high, keyFn, pct, true);
    renderList('rank-low', low, keyFn, pct, false);
  }

  function renderList(id, rows, keyFn, pct, isHigh) {
    const el = document.getElementById(id);
    el.innerHTML = rows.map((r, i) => {
      const v = keyFn(r);
      const p = isHigh ? pct(v) : 100 - pct(v);
      const countyName = COUNTY_NAMES[r.county] || r.county.toUpperCase();
      return `<div class="rank-item">
        <span class="rank-n">${i + 1}</span>
        <span class="rank-name">${titleCase(r.municipality)}</span>
        <span class="rank-county">${countyName}</span>
        <div class="rank-bar-wrap"><div class="rank-bar${isHigh ? '' : ' low'}" style="width:${p}%"></div></div>
        <span class="rank-val">${v.toFixed(2)}%</span>
      </div>`;
    }).join('');
  }

  // ── County bar chart ─────────────────────────────────────────────────────
  function renderCountyBar() {
    const rows2025 = getSkattForYear(2025);
    // average by county
    const byCounty = {};
    rows2025.forEach(r => {
      if (!byCounty[r.county]) byCounty[r.county] = [];
      byCounty[r.county].push(r.kommunal_skatt);
    });
    const countyAvgs = Object.entries(byCounty)
      .map(([c, vals]) => ({ code: c, name: COUNTY_NAMES[c] || c.toUpperCase(), avg: vals.reduce((s, v) => s + v, 0) / vals.length }))
      .sort((a, b) => b.avg - a.avg);

    // Render chart
    const ctx = document.getElementById('county-bar-chart').getContext('2d');
    new Chart(ctx, {
      type: 'bar',
      data: {
        labels: countyAvgs.map(c => c.name),
        datasets: [{
          label: 'Snitt kommunalskatt %',
          data: countyAvgs.map(c => +c.avg.toFixed(2)),
          backgroundColor: countyAvgs.map((_, i) => PALETTE[i % PALETTE.length] + 'cc'),
          borderColor: countyAvgs.map((_, i) => PALETTE[i % PALETTE.length]),
          borderWidth: 1
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false }, tooltip: {
          callbacks: { label: ctx => ` ${ctx.parsed.x.toFixed(2)} %` }
        }},
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => v + '%' }, min: 17 },
          y: { grid: { display: false }, ticks: { color: '#e3ebf5', font: { size: 11 } } }
        }
      }
    });
    document.getElementById('county-bar-chart').style.maxHeight = '480px';

    // Render county ranking list
    const listEl = document.getElementById('county-rank-list');
    const minA = Math.min(...countyAvgs.map(c => c.avg));
    const maxA = Math.max(...countyAvgs.map(c => c.avg));
    listEl.innerHTML = countyAvgs.map((c, i) => {
      const p = Math.round((c.avg - minA) / (maxA - minA) * 100);
      return `<div class="rank-item">
        <span class="rank-n">${i + 1}</span>
        <span class="rank-name">${c.name}</span>
        <div class="rank-bar-wrap"><div class="rank-bar" style="width:${p}%"></div></div>
        <span class="rank-val">${c.avg.toFixed(2)}%</span>
      </div>`;
    }).join('');
  }

  // ── Trend chart ──────────────────────────────────────────────────────────
  function renderTrendPicker() {
    const picksEl = document.getElementById('trend-picks');
    const counties = Object.entries(COUNTY_NAMES).sort((a, b) => a[1].localeCompare(b[1]));
    counties.forEach(([code, name]) => {
      const btn = document.createElement('button');
      btn.textContent = name;
      btn.dataset.code = code;
      btn.style.cssText = `font-size:.75rem;padding:.25rem .6rem;border-radius:6px;cursor:pointer;border:1px solid var(--border2);background:var(--bg2);color:var(--muted);transition:all .15s`;
      if (trendSelected.has(code)) {
        btn.style.background = PALETTE[getCountyIndex(code)] + '33';
        btn.style.borderColor = PALETTE[getCountyIndex(code)];
        btn.style.color = PALETTE[getCountyIndex(code)];
      }
      btn.addEventListener('click', () => {
        if (trendSelected.has(code)) {
          trendSelected.delete(code);
          btn.style.background = 'var(--bg2)';
          btn.style.borderColor = 'var(--border2)';
          btn.style.color = 'var(--muted)';
        } else {
          if (trendSelected.size >= 5) return;
          trendSelected.add(code);
          btn.style.background = PALETTE[getCountyIndex(code)] + '33';
          btn.style.borderColor = PALETTE[getCountyIndex(code)];
          btn.style.color = PALETTE[getCountyIndex(code)];
        }
        renderTrendChart();
      });
      picksEl.appendChild(btn);
    });
  }

  function getCountyIndex(code) {
    return Object.keys(COUNTY_NAMES).indexOf(code);
  }

  function renderTrendChart() {
    const years = [...new Set(allSkatt.map(r => r.year))].sort((a, b) => a - b);

    // Build datasets: average by county × year
    const byCountyYear = {};
    allSkatt.forEach(r => {
      const k = r.county + '|' + r.year;
      if (!byCountyYear[k]) byCountyYear[k] = [];
      byCountyYear[k].push(r.kommunal_skatt);
    });

    const datasets = [...trendSelected].map(code => {
      const color = PALETTE[getCountyIndex(code)];
      return {
        label: COUNTY_NAMES[code] || code,
        data: years.map(y => {
          const vals = byCountyYear[code + '|' + y];
          return vals ? +(vals.reduce((s, v) => s + v, 0) / vals.length).toFixed(2) : null;
        }),
        borderColor: color,
        backgroundColor: color + '33',
        tension: 0.3,
        pointRadius: 4,
        spanGaps: true
      };
    });

    const ctx = document.getElementById('trend-chart').getContext('2d');
    if (trendChart) trendChart.destroy();
    trendChart = new Chart(ctx, {
      type: 'line',
      data: { labels: years, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#e3ebf5', boxWidth: 12 } } },
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0' } },
          y: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => v + '%' } }
        }
      }
    });
    document.getElementById('trend-chart').style.maxHeight = '320px';
  }

  // ── Scatter: income vs tax ───────────────────────────────────────────────
  function renderScatter() {
    // Latest inkomst year by county (metric=mean)
    const inkomstByCounty = {};
    const latestYear = Math.max(...allInkomst.map(r => r.year));
    allInkomst.filter(r => r.year === latestYear && r.metric === 'mean').forEach(r => {
      inkomstByCounty[r.county] = r.value;
    });

    // 2025 avg kommunalskatt per county
    const skatt2025 = getSkattForYear(2025);
    const byCounty = {};
    skatt2025.forEach(r => {
      if (!byCounty[r.county]) byCounty[r.county] = [];
      byCounty[r.county].push(r.kommunal_skatt);
    });

    const points = Object.entries(byCounty).map(([code, vals]) => {
      const avgTax = vals.reduce((s, v) => s + v, 0) / vals.length;
      const income = inkomstByCounty[code];
      if (!income) return null;
      return { x: income, y: +avgTax.toFixed(2), label: COUNTY_NAMES[code] || code };
    }).filter(Boolean);

    const ctx = document.getElementById('scatter-chart').getContext('2d');
    new Chart(ctx, {
      type: 'scatter',
      data: {
        datasets: [{
          label: 'Län',
          data: points,
          backgroundColor: points.map((_, i) => PALETTE[i % PALETTE.length] + 'cc'),
          pointRadius: 8,
          pointHoverRadius: 11
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              label: ctx => {
                const p = ctx.raw;
                return `${p.label}: Inkomst ${p.x} tkr · Skatt ${p.y}%`;
              }
            }
          }
        },
        scales: {
          x: {
            title: { display: true, text: 'Medelinkomst (tkr, ' + latestYear + ')', color: '#6b82a0' },
            grid: { color: '#1b2940' },
            ticks: { color: '#6b82a0', callback: v => v + ' tkr' }
          },
          y: {
            title: { display: true, text: 'Kommunalskatt % (2025)', color: '#6b82a0' },
            grid: { color: '#1b2940' },
            ticks: { color: '#6b82a0', callback: v => v + '%' }
          }
        }
      }
    });
    document.getElementById('scatter-chart').style.maxHeight = '380px';
  }

  // ── Utils ────────────────────────────────────────────────────────────────
  function setText(id, val) {
    const el = document.getElementById(id);
    if (el) el.textContent = val;
  }

  function titleCase(str) {
    return str.toLowerCase().replace(/(?:^|\s|-)(\S)/g, c => c.toUpperCase());
  }

  // ── Företagsöverskott ────────────────────────────────────────────────────
  function initForetagsoverskott() {
    const years = [...new Set(allFoetag.map(r => r.year))].sort((a, b) => b - a);
    const sel = document.getElementById('fo-year');
    years.forEach(y => {
      const o = document.createElement('option');
      o.value = y; o.textContent = y;
      if (y === 2022) o.selected = true;
      sel.appendChild(o);
    });
    sel.addEventListener('change', renderForetagsoverskott);
    renderForetagsoverskott();
  }

  function renderForetagsoverskott() {
    const year = parseInt(document.getElementById('fo-year').value, 10);
    const rows = allFoetag.filter(r => r.year === year)
      .sort((a, b) => b.belopp_sek - a.belopp_sek)
      .slice(0, 20);

    const labels = rows.map(r => titleCase(r.municipality));
    const vals = rows.map(r => +(r.belopp_sek / 1e9).toFixed(1));

    const ctx = document.getElementById('fo-bar-chart').getContext('2d');
    if (foChart) foChart.destroy();
    foChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels,
        datasets: [{
          label: 'Företagsöverskott (Mdr kr)',
          data: vals,
          backgroundColor: PALETTE.map(c => c + 'cc'),
          borderColor: PALETTE,
          borderWidth: 1
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false }, tooltip: {
          callbacks: { label: ctx => ` ${ctx.parsed.x} Mdr kr` }
        }},
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => v + ' Mdr' } },
          y: { grid: { display: false }, ticks: { color: '#e3ebf5', font: { size: 11 } } }
        }
      }
    });
    document.getElementById('fo-bar-chart').style.maxHeight = '400px';

    // Rank list
    const maxV = vals[0];
    document.getElementById('fo-rank-list').innerHTML = rows.map((r, i) => {
      const v = +(r.belopp_sek / 1e9).toFixed(1);
      const p = Math.round(v / maxV * 100);
      return `<div class="rank-item">
        <span class="rank-n">${i + 1}</span>
        <span class="rank-name">${titleCase(r.municipality)}</span>
        <div class="rank-bar-wrap"><div class="rank-bar" style="width:${p}%"></div></div>
        <span class="rank-val">${v} Mdr</span>
      </div>`;
    }).join('');
  }

  // ── Kapitalinkomster ─────────────────────────────────────────────────────
  function initKapitalinkomst() {
    const years = [...new Set(allKapital.map(r => r.year))].sort((a, b) => b - a);
    const kiYearSel = document.getElementById('ki-year');
    years.forEach(y => {
      const o = document.createElement('option');
      o.value = y; o.textContent = y;
      if (y === 2022) o.selected = true;
      kiYearSel.appendChild(o);
    });
    kiYearSel.addEventListener('change', renderKapitalBar);
    document.getElementById('ki-term').addEventListener('change', renderKapitalBar);
    renderKapitalBar();
    renderKapitalTrend();
  }

  function renderKapitalBar() {
    const year = parseInt(document.getElementById('ki-year').value, 10);
    const term = document.getElementById('ki-term').value;
    const rows = allKapital
      .filter(r => r.year === year && r.statistikterm === term)
      .sort((a, b) => b.belopp_sek - a.belopp_sek);

    const ctx = document.getElementById('ki-bar-chart').getContext('2d');
    if (kiBarChart) kiBarChart.destroy();
    kiBarChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels: rows.map(r => r.county_name.charAt(0) + r.county_name.slice(1).toLowerCase()),
        datasets: [{
          label: 'Belopp (Mdr kr)',
          data: rows.map(r => +(r.belopp_sek / 1e9).toFixed(1)),
          backgroundColor: PALETTE.map(c => c + 'cc'),
          borderColor: PALETTE,
          borderWidth: 1
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false }, tooltip: {
          callbacks: { label: ctx => ` ${ctx.parsed.x} Mdr kr` }
        }},
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => v + ' Mdr' } },
          y: { grid: { display: false }, ticks: { color: '#e3ebf5', font: { size: 11 } } }
        }
      }
    });
    document.getElementById('ki-bar-chart').style.maxHeight = '480px';
  }

  function renderKapitalTrend() {
    // Show 4 key income types over time (summed across all counties)
    const termMap = {
      'Ränteinkomster': 'Ränteinkomster, utdelningar, vinst från bilaga K4 avsnitt C m.m.',
      'Fondvinster': 'Vinst fondandelar m.m. Vinst från bilaga K4 avsnitt A och B, K10, K10A, K11, K12 avsnitt B och C och K13',
      'Bostadsvinster': 'Vinst från försäljning av småhus och bostadsrätt, bilaga K5 och K6. Återfört uppskov från bilaga K2.',
      'Schablonintäkter': 'Schablonintäkter'
    };
    const colors = ['#f5a623','#38b6ff','#3fb950','#e05ae0'];
    const years = [...new Set(allKapital.map(r => r.year))].sort((a, b) => a - b);

    const datasets = Object.entries(termMap).map(([label, term], i) => ({
      label,
      data: years.map(y => {
        const rows = allKapital.filter(r => r.year === y && r.statistikterm === term);
        const sum = rows.reduce((s, r) => s + r.belopp_sek, 0);
        return sum > 0 ? +(sum / 1e9).toFixed(1) : null;
      }),
      borderColor: colors[i],
      backgroundColor: colors[i] + '33',
      tension: 0.3,
      pointRadius: 3,
      spanGaps: true
    }));

    const ctx = document.getElementById('ki-trend-chart').getContext('2d');
    if (kiTrendChart) kiTrendChart.destroy();
    kiTrendChart = new Chart(ctx, {
      type: 'line',
      data: { labels: years, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#e3ebf5', boxWidth: 12, font: { size: 11 } } } },
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0' } },
          y: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => v + ' Mdr' } }
        }
      }
    });
    document.getElementById('ki-trend-chart').style.maxHeight = '280px';
  }

  // ── Skatt per person ─────────────────────────────────────────────────────
  function initSkattPerPerson() {
    const years = [...new Set(allSPP.map(r => r.year))].sort((a, b) => b - a);
    const sel = document.getElementById('spp-year');
    years.forEach(y => {
      const o = document.createElement('option');
      o.value = y; o.textContent = y;
      if (y === 2022) o.selected = true;
      sel.appendChild(o);
    });
    sel.addEventListener('change', renderSkattPerPerson);
    document.getElementById('spp-metric').addEventListener('change', renderSkattPerPerson);
    renderSkattPerPerson();
  }

  function renderSkattPerPerson() {
    const year = parseInt(document.getElementById('spp-year').value, 10);
    const metric = document.getElementById('spp-metric').value;
    const field = metric === 'median' ? 'median_kr' : 'medel_kr';
    const label = metric === 'median' ? 'Median direkta skatter (kr)' : 'Medel direkta skatter (kr)';

    let rows = allSPP.filter(r => r.year === year && r.municipality !== 'Uppgift saknas')
      .sort((a, b) => b[field] - a[field]);

    const top20 = rows.slice(0, 20);
    const labels = top20.map(r => titleCase(r.municipality));
    const vals = top20.map(r => r[field]);

    const ctx = document.getElementById('spp-bar-chart').getContext('2d');
    if (sppChart) sppChart.destroy();
    sppChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels,
        datasets: [{
          label,
          data: vals,
          backgroundColor: PALETTE.map(c => c + 'cc'),
          borderColor: PALETTE,
          borderWidth: 1
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false }, tooltip: {
          callbacks: { label: ctx => ` ${ctx.parsed.x.toLocaleString('sv-SE')} kr` }
        }},
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => (v/1000).toFixed(0) + ' tkr' } },
          y: { grid: { display: false }, ticks: { color: '#e3ebf5', font: { size: 11 } } }
        }
      }
    });
    document.getElementById('spp-bar-chart').style.maxHeight = '380px';

    // Rankings
    const maxV = rows[0][field];
    document.getElementById('spp-rank-high').innerHTML = rows.slice(0, 10).map((r, i) => {
      const v = r[field];
      return `<div class="rank-item">
        <span class="rank-n">${i + 1}</span>
        <span class="rank-name">${titleCase(r.municipality)}</span>
        <div class="rank-bar-wrap"><div class="rank-bar" style="width:${Math.round(v/maxV*100)}%"></div></div>
        <span class="rank-val">${(v/1000).toFixed(0)} tkr</span>
      </div>`;
    }).join('');

    const low = rows.filter(r => r[field] > 0).slice(-10);
    document.getElementById('spp-rank-low').innerHTML = low.map((r, i) => {
      const v = r[field];
      return `<div class="rank-item">
        <span class="rank-n" style="color:var(--green)">${i + 1}</span>
        <span class="rank-name">${titleCase(r.municipality)}</span>
        <div class="rank-bar-wrap"><div class="rank-bar low" style="width:${Math.round(v/maxV*100)}%"></div></div>
        <span class="rank-val">${(v/1000).toFixed(0)} tkr</span>
      </div>`;
    }).join('');
  }

  // ── Inkomstnivå ──────────────────────────────────────────────────────────
  function initInkomstniva() {
    const years = [...new Set(allNiva.map(r => r.year))].sort((a, b) => b - a);
    const sel = document.getElementById('niva-year');
    years.forEach(y => {
      const o = document.createElement('option');
      o.value = y; o.textContent = y;
      if (y === 2022) o.selected = true;
      sel.appendChild(o);
    });
    sel.addEventListener('change', renderNivaBar);
    document.getElementById('niva-term').addEventListener('change', renderNivaBar);
    renderNivaBar();
    renderNivaTrend();
  }

  // Sort income brackets in natural order
  function nivaOrder(niva) {
    if (niva === '0 kr') return -1;
    if (niva === '1 kr - 100 tkr') return 0;
    const m = niva.match(/^(\d+)/);
    if (m) return parseInt(m[1], 10);
    if (niva.startsWith('>=')) return 9999;
    return 9998;
  }

  function renderNivaBar() {
    const year = parseInt(document.getElementById('niva-year').value, 10);
    const term = document.getElementById('niva-term').value;
    let rows = allNiva.filter(r => r.year === year && r.statistikterm === term
      && r.inkomstniva !== 'Summering av ej redovisade rader')
      .sort((a, b) => nivaOrder(a.inkomstniva) - nivaOrder(b.inkomstniva));

    const ctx = document.getElementById('niva-bar-chart').getContext('2d');
    if (nivaChart) nivaChart.destroy();
    nivaChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels: rows.map(r => r.inkomstniva),
        datasets: [
          {
            label: 'Belopp (Mdr kr)',
            data: rows.map(r => +(r.belopp_sek / 1e9).toFixed(1)),
            backgroundColor: '#f5a623cc',
            borderColor: '#f5a623',
            borderWidth: 1,
            yAxisID: 'y'
          },
          {
            label: 'Antal (tusen)',
            data: rows.map(r => +(r.antal / 1000).toFixed(0)),
            backgroundColor: '#38b6ff44',
            borderColor: '#38b6ff',
            borderWidth: 1,
            type: 'line',
            yAxisID: 'y2',
            tension: 0.3,
            pointRadius: 4
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#e3ebf5', boxWidth: 12 } } },
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', maxRotation: 35 } },
          y: { grid: { color: '#1b2940' }, ticks: { color: '#f5a623', callback: v => v + ' Mdr' }, position: 'left' },
          y2: { grid: { display: false }, ticks: { color: '#38b6ff', callback: v => v + ' k' }, position: 'right' }
        }
      }
    });
    document.getElementById('niva-bar-chart').style.maxHeight = '320px';
  }

  function renderNivaTrend() {
    const term = 'Kommunal inkomstskatt';
    const years = [...new Set(allNiva.filter(r => r.statistikterm === term).map(r => r.year))].sort((a, b) => a - b);
    const brackets = ['100 - 200 tkr', '200 - 300 tkr', '300 - 400 tkr', '400 - 500 tkr', '>= 1 mnkr'];
    const colors = ['#38b6ff','#3fb950','#f5a623','#e05ae0','#ff7b72'];

    const datasets = brackets.map((b, i) => ({
      label: b,
      data: years.map(y => {
        const r = allNiva.find(d => d.year === y && d.statistikterm === term && d.inkomstniva === b);
        return r ? +(r.belopp_sek / 1e9).toFixed(1) : null;
      }),
      borderColor: colors[i],
      backgroundColor: colors[i] + '33',
      tension: 0.3,
      pointRadius: 3,
      spanGaps: true
    }));

    const ctx = document.getElementById('niva-trend-chart').getContext('2d');
    if (nivaTrendChart) nivaTrendChart.destroy();
    nivaTrendChart = new Chart(ctx, {
      type: 'line',
      data: { labels: years, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { labels: { color: '#e3ebf5', boxWidth: 10, font: { size: 10 } } } },
        scales: {
          x: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0' } },
          y: { grid: { color: '#1b2940' }, ticks: { color: '#6b82a0', callback: v => v + ' Mdr' } }
        }
      }
    });
    document.getElementById('niva-trend-chart').style.maxHeight = '240px';
  }

  init().catch(console.error);
})();
