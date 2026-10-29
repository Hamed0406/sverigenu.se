/* ─── Constants ──────────────────────────────────────────────── */
const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};
const CATEGORIES = [
  { slug:'totalt',         label:'Totalt' },
  { slug:'misshandel',     label:'Misshandel' },
  { slug:'valdtakt',       label:'Våldtäkt' },
  { slug:'frihet_frid',    label:'Frihet & frid' },
  { slug:'inbrott_bostad', label:'Bostadsinbrott' },
  { slug:'ran',            label:'Rån' },
  { slug:'biltillgrepp',   label:'Biltillgrepp' },
  { slug:'skadegorelse',   label:'Skadegörelse' },
  { slug:'narkotika',      label:'Narkotikabrott' },
  { slug:'bedrageri',      label:'Bedrägeri' },
];
const CAT_LABEL = Object.fromEntries(CATEGORIES.map(c=>[c.slug,c.label]));
const PALETTE = ['#e3b341','#38b6ff','#e879a0','#2dd4a0'];

/* ─── State ──────────────────────────────────────────────────── */
let lanData = [];          // /api/v1/brott/lan rows (all categories, all years)
let latestYear = null;
let trendMeasure = 'count';
let trendCats = ['totalt'];
let compareCat = 'totalt';
let trendChart = null;

const fmt = n => n == null ? '—' : Math.round(n).toLocaleString('sv-SE');
const sign = n => n > 0 ? '+' : '';

/* ─── National aggregates from county rows ──────────────────── */
function national(slug) {
  // year -> { count, pop } summed over counties
  const byYear = new Map();
  for (const r of lanData) {
    if (r.category !== slug) continue;
    const e = byYear.get(r.year) || { count:0, pop:0 };
    e.count += r.count;
    if (r.population) e.pop += r.population;
    byYear.set(r.year, e);
  }
  return byYear;
}

/* ─── Fetch ──────────────────────────────────────────────────── */
async function loadData() {
  try {
    const r = await fetch('/api/v1/brott/lan');
    if (!r.ok) throw new Error(r.status);
    lanData = (await r.json()).data || [];
    if (!lanData.length) { showEmpty(); return; }
    latestYear = Math.max(...lanData.map(d=>d.year));
    buildHero();
    buildTrendPicker();
    buildTrendChart();
    buildComparePicker();
    buildLanBars();
    buildKommunBars();
  } catch(e) {
    console.error('brott fetch failed', e);
    showEmpty();
  }
}

function showEmpty() {
  document.getElementById('heroTotalYear').textContent = 'data hämtas — försök igen om en stund';
}

/* ─── Hero ───────────────────────────────────────────────────── */
function buildHero() {
  const tot = national('totalt');
  const now = tot.get(latestYear);
  if (!now) return;
  document.getElementById('heroTotal').textContent = fmt(now.count);
  document.getElementById('heroTotalYear').textContent = String(latestYear);

  if (now.pop > 0) {
    document.getElementById('heroPer100k').textContent = fmt(now.count / now.pop * 100000);
    document.getElementById('heroPer100kSub').textContent = `riket · ${latestYear}`;
  }

  const prev = tot.get(latestYear - 1);
  if (prev) {
    const d = (now.count - prev.count) / prev.count * 100;
    const el = document.getElementById('heroYoY');
    el.textContent = sign(d) + d.toFixed(1) + ' %';
    el.style.color = d > 0 ? 'var(--red)' : 'var(--teal)';
    document.getElementById('heroYoYSub').textContent = `${latestYear-1} → ${latestYear}`;
  }

  const decade = tot.get(latestYear - 10);
  if (decade) {
    const d = (now.count - decade.count) / decade.count * 100;
    const el = document.getElementById('heroDecade');
    el.textContent = sign(d) + d.toFixed(1) + ' %';
    el.style.color = d > 0 ? 'var(--red)' : 'var(--teal)';
    document.getElementById('heroDecadeSub').textContent = `${latestYear-10} → ${latestYear}`;
  }

  // Biggest movers among the non-total categories, latest year vs previous.
  const moves = [];
  for (const c of CATEGORIES) {
    if (c.slug === 'totalt') continue;
    const m = national(c.slug);
    const a = m.get(latestYear), b = m.get(latestYear - 1);
    if (a && b && b.count > 500) moves.push({ ...c, pct: (a.count - b.count) / b.count * 100, now: a.count });
  }
  moves.sort((x,y) => y.pct - x.pct);
  if (moves.length) {
    const up = moves[0], down = moves.at(-1);
    document.getElementById('insightUp').textContent = `${up.label} ${sign(up.pct)}${up.pct.toFixed(1)} %`;
    document.getElementById('insightUpDesc').textContent = `${fmt(up.now)} anmälda brott ${latestYear}`;
    document.getElementById('insightDown').textContent = `${down.label} ${sign(down.pct)}${down.pct.toFixed(1)} %`;
    document.getElementById('insightDownDesc').textContent = `${fmt(down.now)} anmälda brott ${latestYear}`;
  }

  // County with highest total per 100k.
  const counties = lanData.filter(d => d.category === 'totalt' && d.year === latestYear && d.per_100k != null)
    .sort((a,b) => b.per_100k - a.per_100k);
  if (counties.length) {
    const top = counties[0];
    document.getElementById('insightCounty').textContent = COUNTY_NAMES[top.county] || top.county;
    document.getElementById('insightCountyDesc').textContent =
      `${fmt(top.per_100k)} anmälda brott per 100 000 invånare ${latestYear}`;
  }
}

/* ─── Trend picker + chart ───────────────────────────────────── */
function buildTrendPicker() {
  const el = document.getElementById('trendPicker');
  CATEGORIES.forEach(c => {
    const idx = trendCats.indexOf(c.slug);
    const btn = document.createElement('button');
    btn.className = 'pick' + (idx !== -1 ? ' on' : '');
    if (idx !== -1) btn.style.setProperty('--pick-color', PALETTE[idx]);
    btn.textContent = c.label;
    btn.dataset.slug = c.slug;
    btn.addEventListener('click', () => toggleTrendCat(c.slug, btn));
    el.appendChild(btn);
  });
  updateTrendMaxed();
}

function updateTrendMaxed() {
  const atMax = trendCats.length >= 4;
  document.querySelectorAll('#trendPicker .pick').forEach(btn => {
    if (!btn.classList.contains('on')) btn.classList.toggle('maxed', atMax);
  });
}

function toggleTrendCat(slug, btn) {
  const idx = trendCats.indexOf(slug);
  if (idx !== -1) {
    if (trendCats.length <= 1) return;
    trendCats.splice(idx, 1);
    btn.classList.remove('on');
    btn.style.removeProperty('--pick-color');
    document.querySelectorAll('#trendPicker .pick.on').forEach((b,i) =>
      b.style.setProperty('--pick-color', PALETTE[i] || '#888'));
  } else {
    if (trendCats.length >= 4) return;
    trendCats.push(slug);
    btn.classList.add('on');
    btn.style.setProperty('--pick-color', PALETTE[trendCats.length-1] || '#888');
  }
  updateTrendMaxed();
  buildTrendChart();
}

function buildTrendChart() {
  const years = [...new Set(lanData.map(d=>d.year))].sort((a,b)=>a-b);
  const datasets = trendCats.map((slug, i) => {
    const m = national(slug);
    const color = PALETTE[i] || '#888';
    return {
      label: CAT_LABEL[slug],
      data: years.map(y => {
        const e = m.get(y);
        if (!e) return null;
        return trendMeasure === 'count' ? e.count : (e.pop > 0 ? e.count / e.pop * 100000 : null);
      }),
      borderColor: color, backgroundColor: color+'22',
      tension: .3, pointRadius: 2, pointHoverRadius: 5, fill: false, spanGaps: true,
    };
  });

  document.getElementById('trendSub').textContent =
    `hela landet · ${years[0]}–${years.at(-1)} · ${trendMeasure==='count'?'antal':'per 100k'}`;

  const ctx = document.getElementById('trendChart').getContext('2d');
  if (trendChart) trendChart.destroy();
  trendChart = new Chart(ctx, {
    type: 'line',
    data: { labels: years, datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      interaction: { mode:'index', intersect:false },
      plugins: {
        legend: { labels: { color:'#6b82a0', font:{family:'DM Mono',size:11}, boxWidth:16, padding:16 } },
        tooltip: {
          backgroundColor:'#0f1726', borderColor:'#1b2940', borderWidth:1,
          titleColor:'#e3ebf5', bodyColor:'#6b82a0',
          callbacks: { label: c => ` ${c.dataset.label}: ${c.parsed.y == null ? '—' : Math.round(c.parsed.y).toLocaleString('sv-SE')}` }
        }
      },
      scales: {
        x: { ticks:{color:'#3d5270',maxTicksLimit:10}, grid:{color:'rgba(27,41,64,.4)'} },
        y: {
          beginAtZero: true,
          ticks:{color:'#3d5270', callback:v=>v.toLocaleString('sv-SE')},
          grid:{color:'rgba(27,41,64,.4)'},
        }
      }
    }
  });
}

/* ─── Compare picker (single select) ─────────────────────────── */
function buildComparePicker() {
  const el = document.getElementById('comparePicker');
  CATEGORIES.forEach(c => {
    const btn = document.createElement('button');
    btn.className = 'pick' + (c.slug === compareCat ? ' on' : '');
    btn.textContent = c.label;
    btn.dataset.slug = c.slug;
    btn.addEventListener('click', () => {
      compareCat = c.slug;
      el.querySelectorAll('.pick').forEach(b=>b.classList.toggle('on', b.dataset.slug===compareCat));
      buildLanBars();
      buildKommunBars();
    });
    el.appendChild(btn);
  });
}

/* ─── Generic hbar builder ───────────────────────────────────── */
function buildHBars(elId, rows, suffix) {
  const el = document.getElementById(elId);
  el.innerHTML = '';
  if (!rows.length) return;

  const natAvg = rows.reduce((a,r)=>a+r.value,0) / rows.length;
  const max = Math.max(...rows.map(r=>r.value));

  rows.forEach((r, i) => {
    const delta = r.value - natAvg;
    const fillPct = max > 0 ? r.value / max * 100 : 0;
    const avgPct = max > 0 ? natAvg / max * 100 : null;
    const deltaClass = delta > natAvg*.02 ? 'pos' : delta < -natAvg*.02 ? 'neg' : 'neu';

    const row = document.createElement('div');
    row.className = 'hbar-row';
    row.innerHTML = `
      <div class="hbar-rank">${i+1}</div>
      <div class="hbar-name">${r.name}</div>
      <div class="hbar-track">
        <div class="hbar-fill"></div>
        ${avgPct != null ? `<div class="hbar-avg"></div>` : ''}
      </div>
      <div class="hbar-val">${fmt(r.value)}${suffix}</div>
      <div class="hbar-delta ${deltaClass}">${sign(delta)}${fmt(delta)}</div>`;
    el.appendChild(row);

    setTimeout(() => {
      row.querySelector('.hbar-fill').style.width = fillPct + '%';
      const avgEl = row.querySelector('.hbar-avg');
      if (avgEl && avgPct != null) avgEl.style.left = avgPct + '%';
      row.classList.add('in');
    }, 60 + i * 24);
  });
}

/* ─── County bars ────────────────────────────────────────────── */
function buildLanBars() {
  const rows = lanData
    .filter(d => d.category === compareCat && d.year === latestYear && d.per_100k != null)
    .map(d => ({ name: COUNTY_NAMES[d.county] || d.county, value: d.per_100k }))
    .sort((a,b) => b.value - a.value);
  document.getElementById('lanBarTitle').textContent = `${CAT_LABEL[compareCat]} per län · ${latestYear}`;
  document.getElementById('lanBarNote').textContent =
    `Källa: BRÅ + SCB · ${CAT_LABEL[compareCat].toLowerCase()} per 100 000 invånare · ${latestYear}`;
  buildHBars('lanBarList', rows, '');
}

/* ─── Kommun top-20 bars ─────────────────────────────────────── */
async function buildKommunBars() {
  try {
    const r = await fetch(`/api/v1/brott?category=${compareCat}&year=${latestYear}`);
    if (!r.ok) throw new Error(r.status);
    const rows = ((await r.json()).data || [])
      .filter(d => d.per_100k != null && d.count != null && d.count >= 5)
      .map(d => ({ name: d.kommun, value: d.per_100k }))
      .sort((a,b) => b.value - a.value)
      .slice(0, 20);
    document.getElementById('kommunBarTitle').textContent =
      `${CAT_LABEL[compareCat]} · topp 20 kommuner · ${latestYear}`;
    document.getElementById('kommunBarNote').textContent =
      `Källa: BRÅ · per 100 000 invånare i kommunen · ${latestYear}`;
    buildHBars('kommunBarList', rows, '');
  } catch(e) {
    console.error('kommun fetch failed', e);
  }
}

/* ─── Measure toggle ─────────────────────────────────────────── */
document.getElementById('measureToggle').querySelectorAll('.sex-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('#measureToggle .sex-btn').forEach(b=>b.classList.remove('active'));
    btn.classList.add('active');
    trendMeasure = btn.dataset.measure;
    buildTrendChart();
  });
});

loadData();
