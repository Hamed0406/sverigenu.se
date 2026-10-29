'use strict';

const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};

const COUNTY_COLORS = {
  ab:'#56e0a0', o:'#38b6ff', m:'#e3b341', c:'#ff9f43', e:'#e05b6e',
  f:'#2dd4a0', n:'#bc8cff', bd:'#79c0ff', x:'#f78166', y:'#54aeff',
  z:'#3fb950', w:'#daaa3f', d:'#6cb6ff', g:'#57ab5a', h:'#ec8e7b',
  i:'#b083f0', k:'#4fa3e8', s:'#7c6cff', t:'#6fdd8b', u:'#ff7b72', ac:'#8ddb8c'
};

const fmt0 = n => n == null ? '—' : Math.round(n).toLocaleString('sv-SE');
const fmtMW = n => n == null ? '—' : Math.round(n).toLocaleString('sv-SE') + ' MW';

let trendChart = null;
let capacityChart = null;

fetch('/api/v1/vindkraft')
  .then(r => r.json())
  .then(d => {
    const rows = d.data || [];
    if (!rows.length) return;

    // Group by county+year into combined objects
    const byKey = {};
    rows.forEach(r => {
      const key = r.county + '|' + r.year;
      if (!byKey[key]) byKey[key] = { county: r.county, year: r.year };
      byKey[key][r.metric] = r.value;
    });
    const combined = Object.values(byKey);

    // Find latest complete year (exclude partial current year if significantly lower)
    const years = [...new Set(combined.map(r => r.year))].sort();
    const latestYear = years[years.length - 1];
    const prevYear   = years[years.length - 2];

    // Use the year with the most non-null production data as "latest"
    const countNonNull = yr => combined.filter(r => r.year === yr && r.produktion_gwh > 0).length;
    const displayYear  = countNonNull(latestYear) >= countNonNull(prevYear) - 2 ? latestYear : prevYear;

    const latest = combined.filter(r => r.year === displayYear);

    // Hero stats
    const totalGWh = latest.reduce((s, r) => s + (r.produktion_gwh || 0), 0);
    const totalMW  = latest.reduce((s, r) => s + (r.installerad_effekt_mw || 0), 0);
    const topCounty = [...latest].sort((a, b) => (b.produktion_gwh || 0) - (a.produktion_gwh || 0))[0];

    document.getElementById('heroTotal').textContent   = fmt0(totalGWh) + ' GWh';
    document.getElementById('heroYear').textContent    = 'Sverige ' + displayYear;
    document.getElementById('heroHigh').textContent    = fmt0(topCounty?.produktion_gwh) + ' GWh';
    document.getElementById('heroHighName').textContent = COUNTY_NAMES[topCounty?.county] || '';
    document.getElementById('heroMW').textContent      = fmtMW(totalMW);
    document.getElementById('rankingYear').textContent = displayYear;

    buildRanking(latest);
    buildCapacity(latest);
    buildTrend(combined, years);
  });

function buildRanking(latest) {
  const sorted = [...latest].sort((a, b) => (b.produktion_gwh || 0) - (a.produktion_gwh || 0));
  const max = sorted[0]?.produktion_gwh || 1;
  const el = document.getElementById('rankingList');
  el.innerHTML = sorted.map((r, i) => {
    const name = COUNTY_NAMES[r.county] || r.county;
    const gwh = r.produktion_gwh || 0;
    const w = (gwh / max * 100).toFixed(1);
    const col = COUNTY_COLORS[r.county] || '#56e0a0';
    return '<div class="hbar-row">' +
      '<div class="hbar-rank">' + (i + 1) + '</div>' +
      '<div class="hbar-name">' + name + '</div>' +
      '<div class="hbar-track"><div class="hbar-fill" style="width:' + w + '%;background:linear-gradient(90deg,' + col + '44,' + col + ')"></div></div>' +
      '<div class="hbar-val">' + fmt0(gwh) + ' <span style="font-size:.6rem;color:var(--muted)">GWh</span></div>' +
      '</div>';
  }).join('');
}

function buildCapacity(latest) {
  const sorted = [...latest].sort((a, b) => (b.installerad_effekt_mw || 0) - (a.installerad_effekt_mw || 0));
  const labels = sorted.map(r => COUNTY_NAMES[r.county] || r.county);
  const data   = sorted.map(r => r.installerad_effekt_mw || 0);
  const colors = sorted.map(r => COUNTY_COLORS[r.county] || '#56e0a0');

  const ctx = document.getElementById('capacityChart');
  if (capacityChart) capacityChart.destroy();
  capacityChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [{
        label: 'Installerad effekt (MW)',
        data,
        backgroundColor: colors.map(c => c + '99'),
        borderColor: colors,
        borderWidth: 1,
        borderRadius: 3,
      }]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      indexAxis: 'y',
      scales: {
        x: { grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0', callback: v => v + ' MW'} },
        y: { grid:{display:false}, ticks:{color:'#6b82a0', font:{size:11}} }
      },
      plugins: {
        legend:{ display:false },
        tooltip:{ callbacks:{ label: ctx => ' ' + fmt0(ctx.raw) + ' MW' } }
      }
    }
  });
}

function buildTrend(combined, years) {
  // Default: top 5 producers (latest year) + Skåne for variety
  const latestYear = years[years.length - 1];
  const latest = combined.filter(r => r.year === latestYear)
    .sort((a, b) => (b.produktion_gwh || 0) - (a.produktion_gwh || 0));
  const defaultCounties = latest.slice(0, 5).map(r => r.county)
    .concat(['m']).filter((c, i, a) => a.indexOf(c) === i).slice(0, 5);

  renderTrend(combined, years, defaultCounties);

  const sel = document.getElementById('countySelect');
  const sorted = [...latest].sort((a, b) =>
    (COUNTY_NAMES[a.county] || a.county).localeCompare(COUNTY_NAMES[b.county] || b.county, 'sv'));
  sel.innerHTML = sorted.map(r =>
    '<option value="' + r.county + '">' + (COUNTY_NAMES[r.county] || r.county) + '</option>'
  ).join('');
  Array.from(sel.options).forEach(o => { o.selected = defaultCounties.includes(o.value); });
  sel.addEventListener('change', () => {
    const selected = Array.from(sel.selectedOptions).map(o => o.value);
    if (selected.length) renderTrend(combined, years, selected);
  });
}

function renderTrend(combined, years, counties) {
  const datasets = counties.map(code => {
    const map = {};
    combined.filter(r => r.county === code)
            .forEach(r => { map[r.year] = r.produktion_gwh; });
    return {
      label: COUNTY_NAMES[code] || code,
      data: years.map(y => map[y] ?? null),
      borderColor: COUNTY_COLORS[code] || '#56e0a0',
      backgroundColor: 'transparent',
      borderWidth: 2,
      pointRadius: 2,
      tension: 0.35,
      spanGaps: true,
    };
  });

  const ctx = document.getElementById('trendChart');
  if (trendChart) trendChart.destroy();
  trendChart = new Chart(ctx, {
    type: 'line',
    data: { labels: years.map(String), datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      scales: {
        x: { grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0', maxTicksLimit:10} },
        y: {
          grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0', callback: v => fmt0(v)},
          title:{display:true, text:'GWh', color:'#4d647d'}
        }
      },
      plugins: {
        legend:{ labels:{color:'#8da6c0', font:{family:"'DM Mono',monospace", size:11}, boxWidth:12} },
        tooltip:{
          mode:'index', intersect:false,
          callbacks:{ label: ctx => ' ' + ctx.dataset.label + ': ' + fmt0(ctx.raw) + ' GWh' }
        }
      }
    }
  });
}
