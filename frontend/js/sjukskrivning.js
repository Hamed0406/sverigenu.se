'use strict';

const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};
const COUNTY_COLORS = {
  ab:'#4fa3e8', o:'#2dd4a0', m:'#7c6cff', c:'#79c0ff', e:'#d29922',
  f:'#56d364', n:'#ff7b72', bd:'#38b6ff', x:'#bc8cff', y:'#f78166',
  z:'#3fb950', w:'#e3b341', d:'#6cb6ff', g:'#57ab5a', h:'#e0823d',
  i:'#b083f0', k:'#ec8e7b', s:'#54aeff', t:'#6fdd8b', u:'#daaa3f', ac:'#8ddb8c'
};

const fmt1 = n => (n == null ? '—' : n.toFixed(1).replace('.', ','));

let compareData = {};
let compareChart = null;

Promise.all([
  fetch('/api/v1/sjukskrivning').then(r => r.json()),
]).then(([d]) => {
  const counties = d.counties || [];
  const trend    = d.trend    || [];

  if (counties.length) {
    const sorted = [...counties].filter(c => c.andel != null).sort((a,b) => b.andel - a.andel);
    const validCounties = counties.filter(c => c.andel != null);
    const natAvg = validCounties.reduce((s,c) => s + c.andel, 0) / validCounties.length;
    const hi = sorted[0], lo = sorted[sorted.length-1];
    document.getElementById('heroNational').textContent = fmt1(natAvg) + '%';
    if (hi) {
      document.getElementById('heroHigh').textContent = fmt1(hi.andel) + '%';
      document.getElementById('heroHighName').textContent = COUNTY_NAMES[hi.code] || hi.code;
    }
    if (lo) {
      document.getElementById('heroLow').textContent = fmt1(lo.andel) + '%';
      document.getElementById('heroLowName').textContent = COUNTY_NAMES[lo.code] || lo.code;
    }
    if (hi) {
      document.getElementById('rankingSub').textContent =
        hi.year + '-' + String(hi.month).padStart(2,'0');
    }
    buildRanking(sorted);
  }

  if (trend.length) buildTrend(trend);

  buildCountySelect(counties.map(c => c.code));
});

function buildTrend(trend) {
  const ctx = document.getElementById('trendChart');
  const labels = trend.map(p => String(p.year));
  const data   = trend.map(p => p.andel);
  new Chart(ctx, {
    type: 'line',
    data: {
      labels,
      datasets: [{
        label: 'Riket (Dec)',
        data,
        borderColor: '#e05b6e',
        backgroundColor: 'rgba(224,91,110,.12)',
        borderWidth: 2.5,
        pointRadius: 4,
        pointBackgroundColor: '#e05b6e',
        tension: 0.35,
        fill: true,
      }]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      scales: {
        x: { grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0'} },
        y: {
          grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0'},
          title:{display:true,text:'% av pågående sjukfall',color:'#4d647d'}
        }
      },
      plugins: {
        legend:{display:false},
        tooltip:{callbacks:{label: ctx => ' ' + fmt1(ctx.raw) + '% F43'}}
      }
    }
  });
}

function buildRanking(sorted) {
  const max = sorted[0]?.andel || 1;
  const el = document.getElementById('rankingList');
  el.innerHTML = sorted.map((c, i) => {
    const name = COUNTY_NAMES[c.code] || c.code;
    const w = ((c.andel||0) / max * 100).toFixed(1);
    return '<div class="hbar-row">' +
      '<div class="hbar-rank">' + (i+1) + '</div>' +
      '<div class="hbar-name">' + name + '</div>' +
      '<div class="hbar-track"><div class="hbar-fill" style="width:' + w + '%"></div></div>' +
      '<div class="hbar-val">' + fmt1(c.andel) + '%</div>' +
      '</div>';
  }).join('');
}

function buildCountySelect(codes) {
  const sel = document.getElementById('countySelect');
  const sorted = [...codes].sort((a,b) => (COUNTY_NAMES[a]||a).localeCompare(COUNTY_NAMES[b]||b,'sv'));
  sel.innerHTML = sorted.map(c =>
    '<option value="' + c + '">' + (COUNTY_NAMES[c]||c) + '</option>'
  ).join('');

  const defaults = ['ab','m','o','z','y'];
  Array.from(sel.options).forEach(o => { o.selected = defaults.includes(o.value); });

  sel.addEventListener('change', updateCompareChart);
  document.getElementById('selectAllBtn').addEventListener('click', () => {
    Array.from(sel.options).forEach(o => o.selected = true);
    updateCompareChart();
  });
  document.getElementById('clearBtn').addEventListener('click', () => {
    Array.from(sel.options).forEach(o => o.selected = false);
    if (compareChart) { compareChart.destroy(); compareChart = null; }
  });

  updateCompareChart();
}

function updateCompareChart() {
  const sel = document.getElementById('countySelect');
  const selected = Array.from(sel.selectedOptions).map(o => o.value);
  if (!selected.length) return;

  const fetches = selected.filter(c => !compareData[c]).map(c =>
    fetch('/api/v1/sjukskrivning?county=' + c)
      .then(r => r.json())
      .then(d => { compareData[c] = (d.data || []).filter(p => p.year >= 2023); })
  );

  Promise.all(fetches).then(() => renderCompareChart(selected));
}

function renderCompareChart(selected) {
  const ctx = document.getElementById('compareChart');

  const labelSet = new Set();
  selected.forEach(c => {
    (compareData[c] || []).forEach(p => labelSet.add(p.year + '-' + String(p.month).padStart(2,'0')));
  });
  const labels = Array.from(labelSet).sort();

  const datasets = selected.map(c => {
    const map = {};
    (compareData[c] || []).forEach(p => { map[p.year + '-' + String(p.month).padStart(2,'0')] = p.andel; });
    return {
      label: COUNTY_NAMES[c] || c,
      data: labels.map(l => map[l] ?? null),
      borderColor: COUNTY_COLORS[c] || '#888',
      backgroundColor: (COUNTY_COLORS[c] || '#888') + '22',
      borderWidth: 2,
      pointRadius: 2,
      tension: 0.3,
      spanGaps: true,
    };
  });

  if (compareChart) compareChart.destroy();
  compareChart = new Chart(ctx, {
    type: 'line',
    data: { labels, datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      scales: {
        x: { grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0', maxRotation:45, maxTicksLimit:18} },
        y: {
          grid:{color:'rgba(42,63,95,.4)'}, ticks:{color:'#6b82a0'},
          title:{display:true,text:'% F43',color:'#4d647d'}
        }
      },
      plugins: {
        legend:{ labels:{color:'#8da6c0', font:{family:"'DM Mono',monospace",size:11}, boxWidth:12} },
        tooltip:{ mode:'index', intersect:false }
      }
    }
  });
}
