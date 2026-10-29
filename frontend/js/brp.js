'use strict';

const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};

const COUNTY_COLORS = {
  ab:'#e3b341', o:'#56d364', m:'#79c0ff', c:'#ff9f43', e:'#e05b6e',
  f:'#2dd4a0', n:'#bc8cff', bd:'#38b6ff', x:'#f78166', y:'#54aeff',
  z:'#3fb950', w:'#daaa3f', d:'#6cb6ff', g:'#57ab5a', h:'#ec8e7b',
  i:'#b083f0', k:'#4fa3e8', s:'#7c6cff', t:'#6fdd8b', u:'#ff7b72', ac:'#8ddb8c'
};

const fmt0 = n => n == null ? '—' : Math.round(n).toLocaleString('sv-SE');
const fmtPct = n => n == null ? '—' : (n > 0 ? '+' : '') + n.toFixed(1).replace('.',',') + '%';

let trendChart = null;
let allData = [];

fetch('/api/v1/brp')
  .then(r => r.json())
  .then(d => {
    allData = d.data || [];
    if (!allData.length) return;

    // Separate per_capita and growth_pct
    const byCountyYear = {};
    allData.forEach(r => {
      const key = r.county + '|' + r.year;
      if (!byCountyYear[key]) byCountyYear[key] = {county: r.county, year: r.year};
      byCountyYear[key][r.metric] = r.value;
    });
    const rows = Object.values(byCountyYear);

    // Latest year
    const latestYear = Math.max(...rows.map(r => r.year));
    const latest = rows.filter(r => r.year === latestYear && r.per_capita != null);
    latest.sort((a,b) => b.per_capita - a.per_capita);

    // Hero stats
    const natAvg = latest.reduce((s,r) => s + r.per_capita, 0) / latest.length;
    document.getElementById('heroAvg').textContent = fmt0(natAvg) + ' tkr';
    document.getElementById('heroYear').textContent = 'genomsnitt ' + latestYear;
    document.getElementById('heroHigh').textContent = fmt0(latest[0]?.per_capita) + ' tkr';
    document.getElementById('heroHighName').textContent = COUNTY_NAMES[latest[0]?.county] || '';
    document.getElementById('heroLow').textContent = fmt0(latest[latest.length-1]?.per_capita) + ' tkr';
    document.getElementById('heroLowName').textContent = COUNTY_NAMES[latest[latest.length-1]?.county] || '';

    document.getElementById('rankingYear').textContent = latestYear;
    buildRanking(latest);
    buildTrend(rows);
    buildCountySelect(latest.map(r => r.county));
  });

function buildRanking(sorted) {
  const max = sorted[0]?.per_capita || 1;
  const el = document.getElementById('rankingList');
  el.innerHTML = sorted.map((r, i) => {
    const name = COUNTY_NAMES[r.county] || r.county;
    const w = (r.per_capita / max * 100).toFixed(1);
    const growth = r.growth_pct != null
      ? '<span style="font-family:\'DM Mono\',monospace;font-size:.62rem;color:' + (r.growth_pct>=0?'#3fb950':'#ff7b72') + '">' + fmtPct(r.growth_pct) + '</span>'
      : '';
    return '<div class="hbar-row">' +
      '<div class="hbar-rank">' + (i+1) + '</div>' +
      '<div class="hbar-name">' + name + '</div>' +
      '<div class="hbar-track"><div class="hbar-fill" style="width:' + w + '%;background:linear-gradient(90deg,' +
        (COUNTY_COLORS[r.county]||'#888') + '44,' + (COUNTY_COLORS[r.county]||'#888') + ')"></div></div>' +
      '<div class="hbar-val">' + fmt0(r.per_capita) + ' <span style="font-size:.6rem;color:var(--muted)">tkr</span>' + growth + '</div>' +
      '</div>';
  }).join('');
}

function buildTrend(rows) {
  // Pre-select: top 3 + bottom 3 counties by latest BRP
  const latestYear = Math.max(...rows.map(r => r.year));
  const latest = rows.filter(r => r.year === latestYear && r.per_capita != null)
    .sort((a,b) => b.per_capita - a.per_capita);
  const defaultCounties = [
    latest[0]?.county, latest[1]?.county,
    latest[latest.length-1]?.county, latest[latest.length-2]?.county,
    'o', 'm'
  ].filter((c,i,a) => c && a.indexOf(c) === i).slice(0, 5);

  renderTrend(rows, defaultCounties);

  // Populate select
  const sel = document.getElementById('countySelect');
  const sorted = [...latest].sort((a,b) =>
    (COUNTY_NAMES[a.county]||a.county).localeCompare(COUNTY_NAMES[b.county]||b.county,'sv'));
  sel.innerHTML = sorted.map(r =>
    '<option value="' + r.county + '">' + (COUNTY_NAMES[r.county]||r.county) + '</option>'
  ).join('');
  Array.from(sel.options).forEach(o => { o.selected = defaultCounties.includes(o.value); });
  sel.addEventListener('change', () => {
    const selected = Array.from(sel.selectedOptions).map(o => o.value);
    renderTrend(rows, selected);
  });
}

function renderTrend(rows, counties) {
  const years = [...new Set(rows.map(r => r.year))].sort();
  const datasets = counties.map(code => {
    const map = {};
    rows.filter(r => r.county === code && r.per_capita != null)
        .forEach(r => { map[r.year] = r.per_capita; });
    return {
      label: COUNTY_NAMES[code] || code,
      data: years.map(y => map[y] ?? null),
      borderColor: COUNTY_COLORS[code] || '#888',
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
        x: { grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0'} },
        y: {
          grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0'},
          title:{display:true, text:'tkr per invånare', color:'#4d647d'}
        }
      },
      plugins: {
        legend:{ labels:{color:'#8da6c0', font:{family:"'DM Mono',monospace", size:11}, boxWidth:12} },
        tooltip:{
          mode:'index', intersect:false,
          callbacks:{ label: ctx => ' ' + COUNTY_NAMES[ctx.dataset.label] || ctx.dataset.label +
            ': ' + fmt0(ctx.raw) + ' tkr' }
        }
      }
    }
  });
}

function buildCountySelect(codes) {
  // already done in buildTrend; this is a no-op placeholder
}
