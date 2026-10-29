/* ─── Constants ──────────────────────────────────────────── */
const COURT_TYPE_NAMES = {
  'District Courts':                 'Tingsrätter',
  'Courts of Appeal':                'Hovrätter',
  'Supreme Court':                   'Högsta domstolen',
  'Administrative Courts':           'Förvaltningsrätter',
  'Administrative Courts of appeal': 'Kammarrätter',
  'Supreme Administrative Court':    'Högsta förvaltningsdomstolen',
  'Migration Courts':                'Migrationsdomstolar',
  'Migration Court of Appeal':       'Migrationsöverdomstolen',
};

const COURT_COLORS = [
  '#7c6cff','#38b6ff','#2dd4a0','#e3b341','#e879a0',
  '#ff7b72','#9cdcfe','#ce9178',
];

/* ─── State ──────────────────────────────────────────────── */
let trendData = [];
let procData  = [];
let trendMetric = 'filed';
let activeCourts = new Set(['District Courts','Courts of Appeal','Administrative Courts','Migration Courts']);
let caseType = 'proc_crim';
let procYear = 2025;
let trendChart = null, procCrimChart = null, procCivilChart = null;

/* ─── Load ───────────────────────────────────────────────── */
async function loadAll() {
  const [tRes, pRes] = await Promise.all([
    fetch('/api/v1/domstol/trends'),
    fetch('/api/v1/domstol/processing'),
  ]);
  trendData = (await tRes.json()).data || [];
  procData  = (await pRes.json()).data || [];
  buildPage();
}

function buildPage() {
  buildHero();
  buildCourtChips();
  buildTrendChart();
  buildProcBarYear();
  buildProcBars();
  buildProcTrendCharts();
}

/* ─── Hero ───────────────────────────────────────────────── */
function buildHero() {
  // Filed & pending for tingsrätter 2025 and 2024
  function trVal(metric, year) {
    const r = trendData.find(d => d.court_code==='District Courts' && d.metric===metric && d.year===year);
    return r ? r.value : null;
  }
  const filed25 = trVal('filed', 2025);
  const filed24 = trVal('filed', 2024);
  const pend25  = trVal('pending', 2025);
  const pend24  = trVal('pending', 2024);

  if (filed25) document.getElementById('heroFiled').textContent = Math.round(filed25).toLocaleString('sv');
  if (filed25 && filed24) {
    const delta = filed25 - filed24;
    const el = document.getElementById('heroFiledDelta');
    el.textContent = `${delta > 0 ? '+' : ''}${Math.round(delta).toLocaleString('sv')} vs 2024`;
    el.className = 'band-sub ' + (delta > 0 ? 'up' : 'down');
  }
  if (pend25) document.getElementById('heroPending').textContent = Math.round(pend25).toLocaleString('sv');
  if (pend25 && pend24) {
    const delta = pend25 - pend24;
    const el = document.getElementById('heroPendingDelta');
    el.textContent = `${delta > 0 ? '+' : ''}${Math.round(delta).toLocaleString('sv')} vs 2024`;
    el.className = 'band-sub ' + (delta > 0 ? 'up' : 'down');
  }

  // Processing time for "Alla tingsrätter" 2025
  function procAgg(metric, year) {
    const r = procData.find(d => d.court_code==='All district courts' && d.metric===metric && d.year===year);
    return r ? r.value : null;
  }
  const pc = procAgg('proc_crim', 2025);
  const pv = procAgg('proc_civil', 2025);
  if (pc) document.getElementById('heroProcCrim').textContent = pc.toFixed(1) + ' mån';
  if (pv) document.getElementById('heroProcCivil').textContent = pv.toFixed(1) + ' mån';

  // Trend: filed 2010 vs 2025
  const f10 = trVal('filed', 2010);
  if (filed25 && f10) {
    const delta = filed25 - f10;
    const el = document.getElementById('insightFiledTrend');
    el.textContent = `${delta > 0 ? '+' : ''}${Math.round(delta).toLocaleString('sv')} mål`;
    el.style.color = delta > 0 ? 'var(--red)' : 'var(--teal)';
    document.getElementById('insightFiledDesc').textContent =
      `sedan 2010 (${Math.round(f10).toLocaleString('sv')} → ${Math.round(filed25).toLocaleString('sv')})`;
  }

  // Fastest / slowest tingsrätt 2025 (brottmål, excl aggregate)
  const courts2025 = procData.filter(d => d.metric==='proc_crim' && d.year===2025 && d.court_code!=='All district courts');
  if (courts2025.length) {
    const sorted = [...courts2025].sort((a,b)=>a.value-b.value);
    const fastest = sorted[0];
    const slowest = sorted.at(-1);
    document.getElementById('insightFastest').textContent = fastest.value.toFixed(1) + ' mån';
    document.getElementById('insightFastestDesc').textContent = fastest.court_name;
    document.getElementById('insightSlowest').textContent = slowest.value.toFixed(1) + ' mån';
    document.getElementById('insightSlowestDesc').textContent = slowest.court_name;
  }
}

/* ─── Court type chips ───────────────────────────────────── */
function buildCourtChips() {
  const el = document.getElementById('courtChips');
  Object.entries(COURT_TYPE_NAMES).forEach(([code, name], i) => {
    const chip = document.createElement('button');
    chip.className = 'leg-chip' + (activeCourts.has(code) ? ' active' : '');
    chip.style.setProperty('--chip-color', COURT_COLORS[i] || '#888');
    chip.innerHTML = `<span class="dot" style="background:${COURT_COLORS[i]}"></span>${name}`;
    chip.addEventListener('click', () => {
      if (activeCourts.has(code)) {
        if (activeCourts.size <= 1) return;
        activeCourts.delete(code);
        chip.classList.remove('active');
      } else {
        activeCourts.add(code);
        chip.classList.add('active');
      }
      buildTrendChart();
    });
    el.appendChild(chip);
  });
}

/* ─── Case volume trend chart ────────────────────────────── */
function buildTrendChart() {
  const rows = trendData.filter(d => d.metric === trendMetric);
  const years = [...new Set(rows.map(d=>d.year))].sort();

  const datasets = [...activeCourts].map((code, i) => {
    const idx = Object.keys(COURT_TYPE_NAMES).indexOf(code);
    const color = COURT_COLORS[idx >= 0 ? idx : i] || '#888';
    return {
      label: COURT_TYPE_NAMES[code] || code,
      data: years.map(y => rows.find(d=>d.court_code===code && d.year===y)?.value ?? null),
      borderColor: color, backgroundColor: color+'22',
      tension: .3, pointRadius: 2, pointHoverRadius: 5, fill: false, spanGaps: true,
    };
  });

  const metricLabel = {filed:'Inkomna mål',determined:'Avgjorda mål',pending:'Utgående balans'}[trendMetric];
  document.getElementById('trendSub').textContent = `${metricLabel} · 2002–2025`;

  const ctx = document.getElementById('trendChart').getContext('2d');
  if (trendChart) trendChart.destroy();
  trendChart = new Chart(ctx, {
    type: 'line',
    data: { labels: years, datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      interaction: { mode:'index', intersect:false },
      plugins: {
        legend: { labels: { color:'#6b82a0', font:{family:'DM Mono',size:11}, boxWidth:16, padding:14 } },
        tooltip: {
          backgroundColor:'#0f1726', borderColor:'#1b2940', borderWidth:1,
          titleColor:'#e3ebf5', bodyColor:'#6b82a0',
          callbacks: { label: c => ` ${c.dataset.label}: ${Math.round(c.parsed.y).toLocaleString('sv')} mål` }
        }
      },
      scales: {
        x: { ticks:{color:'#3d5270',maxTicksLimit:10}, grid:{color:'rgba(27,41,64,.4)'} },
        y: { ticks:{color:'#3d5270',callback:v=>Math.round(v/1000)+'k'}, grid:{color:'rgba(27,41,64,.4)'} },
      }
    }
  });
}

/* ─── Build year selector for processing bars ────────────── */
function buildProcBarYear() {
  const years = [...new Set(procData.map(d=>d.year))].sort((a,b)=>b-a);
  procYear = years[0] ?? 2025;
  const sel = document.getElementById('procYear');
  years.forEach(y => {
    const opt = document.createElement('option');
    opt.value = y; opt.textContent = y;
    if (y === procYear) opt.selected = true;
    sel.appendChild(opt);
  });
  sel.addEventListener('change', () => { procYear = +sel.value; buildProcBars(); });
}

/* ─── Processing time horizontal bars ───────────────────── */
function buildProcBars() {
  const rows = procData.filter(d =>
    d.metric === caseType &&
    d.year === procYear &&
    d.court_code !== 'All district courts'
  ).sort((a,b) => b.value - a.value);

  const metricLabel = caseType==='proc_crim' ? 'brottmål' : 'tvistemål';
  document.getElementById('procBarSub').textContent = `${procYear} · ${metricLabel} · p75`;
  document.getElementById('procBarNote').textContent =
    `Källa: Domstolsverket · ${procYear} · handläggningstid 75-percentil i månader · ${metricLabel}`;

  const natAvg = rows.reduce((s,r)=>s+r.value,0) / (rows.length||1);
  const maxVal = Math.max(...rows.map(r=>r.value));
  const colorFrom = caseType==='proc_crim' ? 'rgba(227,179,65,.35)' : 'rgba(56,182,255,.35)';
  const colorTo   = caseType==='proc_crim' ? 'var(--gold)' : 'var(--aurora-b)';

  const el = document.getElementById('procBarList');
  el.innerHTML = '';
  rows.forEach((r, i) => {
    const delta = r.value - natAvg;
    const pct   = maxVal > 0 ? (r.value / maxVal * 100) : 50;
    const avgPct = maxVal > 0 ? (natAvg / maxVal * 100) : 50;
    const deltaClass = delta > 0.15 ? 'neg' : delta < -0.15 ? 'pos' : 'neu';
    // For processing time: above avg = bad (red), below avg = good (teal)
    const deltaColor = delta > 0.15 ? 'var(--red)' : delta < -0.15 ? 'var(--teal)' : 'var(--dim)';

    const row = document.createElement('div');
    row.className = 'hbar-row';
    row.innerHTML = `
      <div class="hbar-rank">${i+1}</div>
      <div class="hbar-name" title="${r.court_name}">${r.court_name.replace(' tingsrätt','')}</div>
      <div class="hbar-track">
        <div class="hbar-fill" style="--bar-from:${colorFrom};--bar-to:${colorTo}"></div>
        <div class="hbar-avg-line"></div>
      </div>
      <div class="hbar-val" style="color:${deltaColor}">${r.value.toFixed(1)} mån</div>`;
    el.appendChild(row);
    setTimeout(() => {
      row.querySelector('.hbar-fill').style.width = pct + '%';
      row.querySelector('.hbar-avg-line').style.left = avgPct + '%';
      row.classList.add('in');
    }, 60 + i * 22);
  });
}

/* ─── Processing time trend charts ───────────────────────── */
function buildProcTrendCharts() {
  const buildOne = (canvasId, metric, color) => {
    const rows = procData.filter(d => d.metric===metric && d.court_code==='All district courts');
    const years = [...new Set(rows.map(d=>d.year))].sort();
    const ctx = document.getElementById(canvasId).getContext('2d');
    return new Chart(ctx, {
      type: 'line',
      data: {
        labels: years,
        datasets: [{
          label: metric==='proc_crim' ? 'Brottmål' : 'Tvistemål',
          data: years.map(y => rows.find(d=>d.year===y)?.value ?? null),
          borderColor: color, backgroundColor: color+'22',
          tension: .35, pointRadius: 3, pointHoverRadius: 6, fill: true, spanGaps: true,
        }]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor:'#0f1726', borderColor:'#1b2940', borderWidth:1,
            titleColor:'#e3ebf5', bodyColor:'#6b82a0',
            callbacks: { label: c => ` ${c.parsed.y?.toFixed(1) ?? '—'} månader` }
          }
        },
        scales: {
          x: { ticks:{color:'#3d5270',maxTicksLimit:8}, grid:{color:'rgba(27,41,64,.4)'} },
          y: { ticks:{color:'#3d5270',callback:v=>v+' mån'}, grid:{color:'rgba(27,41,64,.4)'}, min:2 },
        }
      }
    });
  };
  procCrimChart  = buildOne('procCrimChart',  'proc_crim',  '#e3b341');
  procCivilChart = buildOne('procCivilChart', 'proc_civil', '#38b6ff');
}

/* ─── Toggles ────────────────────────────────────────────── */
document.getElementById('metricToggle').querySelectorAll('.tog-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.getElementById('metricToggle').querySelectorAll('.tog-btn').forEach(b=>b.classList.remove('active'));
    btn.classList.add('active');
    trendMetric = btn.dataset.metric;
    buildTrendChart();
  });
});

document.getElementById('caseTypeToggle').querySelectorAll('.tog-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.getElementById('caseTypeToggle').querySelectorAll('.tog-btn').forEach(b=>b.classList.remove('active'));
    btn.classList.add('active');
    caseType = btn.dataset.ct;
    buildProcBars();
  });
});

loadAll();
