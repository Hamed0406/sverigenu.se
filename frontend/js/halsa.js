/* ─── Constants ──────────────────────────────────────────────── */
const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};
const COUNTY_ORDER = Object.keys(COUNTY_NAMES);
const PALETTE = ['#2dd4a0','#38b6ff','#e3b341','#e879a0','#7c6cff'];

/* ─── State ──────────────────────────────────────────────────── */
let allData = [];
let lifeSeriesSex = 'total';
let lifeBarSex    = 'total';
let smokeSex      = 'total';
let alcoholSex    = 'total';
let selectedCounties = ['ab','o','m'];
let lifeChart = null;

/* ─── Helpers ────────────────────────────────────────────────── */
function latestPeriod(rows) {
  return [...new Set(rows.map(d=>d.period))].sort().at(-1);
}
function prevPeriod(rows) {
  const sorted = [...new Set(rows.map(d=>d.period))].sort();
  return sorted.length > 1 ? sorted.at(-2) : null;
}
function avg(vals) {
  return vals.length ? vals.reduce((a,b)=>a+b,0)/vals.length : null;
}
function sign(n) { return n > 0 ? '+' : ''; }

/* ─── Fetch ──────────────────────────────────────────────────── */
async function loadData() {
  try {
    const r = await fetch('/api/v1/folkhalsa');
    if (!r.ok) throw new Error(r.status);
    allData = (await r.json()).data || [];
    buildPage();
  } catch(e) {
    console.error('folkhalsa fetch failed', e);
  }
}

/* ─── Build all ─────────────────────────────────────────────── */
function buildPage() {
  buildHero();
  buildCountyPicker();
  buildLifeChart();
  buildLifeBars(lifeBarSex);
  buildSmokeBars(smokeSex);
  buildAlcoholBars(alcoholSex);
}

/* ─── Hero + insights ────────────────────────────────────────── */
function buildHero() {
  // ── Life expectancy ──
  const life = allData.filter(d => d.metric==='lifeexp');
  if (life.length) {
    const latest = latestPeriod(life);
    const totalLatest = life.filter(d=>d.period===latest && d.sex==='total');
    const natAvg = avg(totalLatest.map(d=>d.value));
    if (natAvg) document.getElementById('heroLifeTotal').textContent = natAvg.toFixed(1) + ' år';
    document.getElementById('heroLifeYear').textContent = latest;

    // Best & worst county
    const sorted = [...totalLatest].sort((a,b)=>b.value-a.value);
    if (sorted.length) {
      const best  = sorted[0];
      const worst = sorted.at(-1);
      document.getElementById('heroBest').textContent = best.value.toFixed(1) + ' år';
      document.getElementById('heroBestCounty').textContent = COUNTY_NAMES[best.county] || best.county;
      document.getElementById('heroWorst').textContent = worst.value.toFixed(1) + ' år';
      document.getElementById('heroWorstCounty').textContent = COUNTY_NAMES[worst.county] || worst.county;
    }

    // Gender gap
    const wAvg = avg(life.filter(d=>d.period===latest && d.sex==='women').map(d=>d.value));
    const mAvg = avg(life.filter(d=>d.period===latest && d.sex==='men').map(d=>d.value));
    if (wAvg && mAvg) {
      const gap = wAvg - mAvg;
      document.getElementById('heroGap').textContent = gap.toFixed(1) + ' år';
    }

    // Trend since 2006
    const oldest = life.filter(d=>d.period==='2006' && d.sex==='total');
    const oldAvg = avg(oldest.map(d=>d.value));
    if (oldAvg && natAvg) {
      const delta = natAvg - oldAvg;
      const el = document.getElementById('insightTrend');
      el.textContent = sign(delta) + delta.toFixed(1) + ' år';
      el.className = 'ic-val ' + (delta >= 0 ? 'ic-trend-up' : 'ic-trend-down');
      document.getElementById('insightTrendDesc').textContent =
        `sedan 2006 (${oldAvg.toFixed(1)} → ${natAvg.toFixed(1)} år)`;
    }
  }

  // ── Smoking ──
  const smoke = allData.filter(d=>d.metric==='smoking' && d.sex==='total');
  if (smoke.length) {
    const sl = latestPeriod(smoke);
    const sp = prevPeriod(smoke);
    const sv = avg(smoke.filter(d=>d.period===sl).map(d=>d.value));
    const pv = sp ? avg(smoke.filter(d=>d.period===sp).map(d=>d.value)) : null;
    if (sv != null) {
      document.getElementById('insightSmoke').textContent = sv.toFixed(1) + ' %';
      const delta = pv != null ? sv - pv : null;
      document.getElementById('insightSmokeDesc').textContent =
        delta != null
          ? `${sl} · ${sign(delta)}${delta.toFixed(1)} % jämfört med ${sp}`
          : `${sl} · rikssnitt`;
    }
  }

  // ── Alcohol ──
  const alc = allData.filter(d=>d.metric==='alcohol' && d.sex==='total');
  if (alc.length) {
    const al = latestPeriod(alc);
    const ap = prevPeriod(alc);
    const av2 = avg(alc.filter(d=>d.period===al).map(d=>d.value));
    const pv2 = ap ? avg(alc.filter(d=>d.period===ap).map(d=>d.value)) : null;
    if (av2 != null) {
      document.getElementById('insightAlcohol').textContent = av2.toFixed(1) + ' %';
      const delta = pv2 != null ? av2 - pv2 : null;
      document.getElementById('insightAlcoholDesc').textContent =
        delta != null
          ? `${al} · ${sign(delta)}${delta.toFixed(1)} % jämfört med ${ap}`
          : `${al} · rikssnitt`;
    }
  }
}

/* ─── County picker ──────────────────────────────────────────── */
function buildCountyPicker() {
  const el = document.getElementById('countyPicker');
  COUNTY_ORDER.forEach(code => {
    const idx = selectedCounties.indexOf(code);
    const btn = document.createElement('button');
    btn.className = 'pick' + (idx !== -1 ? ' on' : '');
    if (idx !== -1) btn.style.setProperty('--pick-color', PALETTE[idx]);
    btn.textContent = COUNTY_NAMES[code];
    btn.dataset.county = code;
    btn.addEventListener('click', () => toggleCounty(code, btn));
    el.appendChild(btn);
  });
  updatePickerMaxed();
}

function updatePickerMaxed() {
  const atMax = selectedCounties.length >= 5;
  document.querySelectorAll('#countyPicker .pick').forEach(btn => {
    if (!btn.classList.contains('on')) {
      btn.classList.toggle('maxed', atMax);
    }
  });
}

function toggleCounty(code, btn) {
  const idx = selectedCounties.indexOf(code);
  if (idx !== -1) {
    if (selectedCounties.length <= 1) return;
    selectedCounties.splice(idx, 1);
    btn.classList.remove('on');
    btn.style.removeProperty('--pick-color');
    // Reassign colors to remaining buttons
    document.querySelectorAll('#countyPicker .pick.on').forEach((b, i) => {
      b.style.setProperty('--pick-color', PALETTE[i] || '#888');
    });
  } else {
    if (selectedCounties.length >= 5) return;
    selectedCounties.push(code);
    btn.classList.add('on');
    btn.style.setProperty('--pick-color', PALETTE[selectedCounties.length-1] || '#888');
  }
  updatePickerMaxed();
  buildLifeChart();
}

/* ─── Life expectancy time-series chart ──────────────────────── */
function buildLifeChart() {
  const rows  = allData.filter(d => d.metric==='lifeexp' && d.sex===lifeSeriesSex);
  const xLabels = [...new Set(rows.map(d=>d.period))].sort();

  const datasets = selectedCounties.map((code, i) => {
    const color = PALETTE[i] || '#888';
    return {
      label: COUNTY_NAMES[code] || code,
      data: xLabels.map(p => rows.find(d=>d.county===code && d.period===p)?.value ?? null),
      borderColor: color, backgroundColor: color+'22',
      tension: .35, pointRadius: 2, pointHoverRadius: 5, fill: false, spanGaps: true,
    };
  });

  const sexLabel = {total:'Totalt',women:'Kvinnor',men:'Män'}[lifeSeriesSex];
  document.getElementById('lifeSeriesSub').textContent =
    `${sexLabel} · ${selectedCounties.length} ${selectedCounties.length===1?'län':'länvalda'}`;

  const ctx = document.getElementById('lifeChart').getContext('2d');
  if (lifeChart) lifeChart.destroy();
  lifeChart = new Chart(ctx, {
    type: 'line',
    data: { labels: xLabels, datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      interaction: { mode:'index', intersect:false },
      plugins: {
        legend: { labels: { color:'#6b82a0', font:{family:'DM Mono',size:11}, boxWidth:16, padding:16 } },
        tooltip: {
          backgroundColor:'#0f1726', borderColor:'#1b2940', borderWidth:1,
          titleColor:'#e3ebf5', bodyColor:'#6b82a0',
          callbacks: { label: c => ` ${c.dataset.label}: ${c.parsed.y?.toFixed(1)??'—'} år` }
        }
      },
      scales: {
        x: { ticks:{color:'#3d5270',maxTicksLimit:8}, grid:{color:'rgba(27,41,64,.4)'} },
        y: {
          ticks:{color:'#3d5270', callback:v=>v+' år'},
          grid:{color:'rgba(27,41,64,.4)'}, min:78,
        }
      }
    }
  });
}

/* ─── Generic hbar builder ───────────────────────────────────── */
function buildHBars(elId, rows, opts) {
  // opts: { colorFrom, colorTo, valueSuffix, baseline, noteEl, noteText }
  const el = document.getElementById(elId);
  if (!rows.length) return;

  const baseline = opts.baseline ?? 0;
  const natAvg   = avg(rows.map(r=>r.value));
  const maxRange = Math.max(...rows.map(r=>r.value)) - baseline;

  el.innerHTML = '';
  rows.forEach((r, i) => {
    const delta  = natAvg != null ? r.value - natAvg : null;
    const fillPct = maxRange > 0 ? ((r.value - baseline) / maxRange * 100) : 50;
    const avgPct  = natAvg != null && maxRange > 0
      ? ((natAvg - baseline) / maxRange * 100) : null;

    const deltaClass = delta == null ? 'neu' : delta > 0.05 ? 'pos' : delta < -0.05 ? 'neg' : 'neu';
    const deltaText  = delta == null ? '' : `${sign(delta)}${delta.toFixed(1)}`;

    const row = document.createElement('div');
    row.className = 'hbar-row';
    row.innerHTML = `
      <div class="hbar-rank">${i+1}</div>
      <div class="hbar-name">${r.name}</div>
      <div class="hbar-track">
        <div class="hbar-fill" style="--bar-from:${opts.colorFrom};--bar-to:${opts.colorTo}"></div>
        ${avgPct != null ? `<div class="hbar-avg"></div>` : ''}
      </div>
      <div class="hbar-val">${r.value.toFixed(1)}${opts.valueSuffix}</div>
      <div class="hbar-delta ${deltaClass}">${deltaText}</div>`;
    el.appendChild(row);

    setTimeout(() => {
      row.querySelector('.hbar-fill').style.width = fillPct + '%';
      const avgEl = row.querySelector('.hbar-avg');
      if (avgEl && avgPct != null) avgEl.style.left = avgPct + '%';
      row.classList.add('in');
    }, 60 + i * 28);
  });

  if (opts.noteEl) {
    document.getElementById(opts.noteEl).textContent = opts.noteText || '';
  }
}

/* ─── Life expectancy bars ───────────────────────────────────── */
function buildLifeBars(sex) {
  const life = allData.filter(d => d.metric==='lifeexp' && d.sex===sex);
  if (!life.length) return;
  const latest = latestPeriod(life);
  const label  = {total:'Totalt',women:'Kvinnor',men:'Män'}[sex];
  document.getElementById('lifeBarSub').textContent = `${latest} · ${label}`;

  const colorFrom = sex==='women' ? 'rgba(232,121,160,.35)' :
                    sex==='men'   ? 'rgba(56,182,255,.35)' : 'rgba(45,212,160,.35)';
  const colorTo   = sex==='women' ? 'var(--pink)' : sex==='men' ? 'var(--blue)' : 'var(--teal)';

  const rows = COUNTY_ORDER
    .map(code => {
      const row = life.find(d=>d.county===code && d.period===latest);
      return row ? { code, name: COUNTY_NAMES[code]||code, value: row.value } : null;
    })
    .filter(Boolean)
    .sort((a,b) => b.value - a.value);

  buildHBars('lifeBarList', rows, {
    colorFrom, colorTo, valueSuffix: ' år', baseline: 79,
    noteEl: 'lifeBarNote',
    noteText: `Källa: Folkhälsomyndigheten · ${latest} · medellivslängd vid födseln · stapeln börjar vid 79 år`,
  });
}

/* ─── Smoking bars ───────────────────────────────────────────── */
function buildSmokeBars(sex) {
  const rows = allData.filter(d => d.metric==='smoking' && d.sex===sex);
  if (!rows.length) return;
  const latest = latestPeriod(rows);
  const prev   = prevPeriod(rows);

  const colorFrom = sex==='women' ? 'rgba(232,121,160,.35)' :
                    sex==='men'   ? 'rgba(56,182,255,.35)' : 'rgba(227,179,65,.35)';
  const colorTo   = sex==='women' ? 'var(--pink)' : sex==='men' ? 'var(--blue)' : 'var(--gold)';

  const data = COUNTY_ORDER
    .map(code => {
      const r = rows.find(d=>d.county===code && d.period===latest);
      return r ? { code, name: COUNTY_NAMES[code]||code, value: r.value } : null;
    })
    .filter(Boolean)
    .sort((a,b) => b.value - a.value);

  const natAvgPrev = prev ? avg(rows.filter(d=>d.period===prev).map(d=>d.value)) : null;
  const natAvgNow  = avg(data.map(d=>d.value));
  const trendNote  = natAvgPrev && natAvgNow
    ? ` · trend: ${sign(natAvgNow-natAvgPrev)}${(natAvgNow-natAvgPrev).toFixed(1)} pp jämfört med ${prev}`
    : '';

  buildHBars('smokeBarList', data, {
    colorFrom, colorTo, valueSuffix: ' %', baseline: 0,
    noteEl: 'smokeNote',
    noteText: `Källa: Folkhälsomyndigheten · ${latest}${trendNote}`,
  });
}

/* ─── Alcohol bars ───────────────────────────────────────────── */
function buildAlcoholBars(sex) {
  const rows = allData.filter(d => d.metric==='alcohol' && d.sex===sex);
  if (!rows.length) return;
  const latest = latestPeriod(rows);
  const prev   = prevPeriod(rows);

  const colorFrom = sex==='women' ? 'rgba(232,121,160,.35)' :
                    sex==='men'   ? 'rgba(56,182,255,.35)' : 'rgba(124,108,255,.35)';
  const colorTo   = sex==='women' ? 'var(--pink)' : sex==='men' ? 'var(--blue)' : 'var(--aurora-v)';

  const data = COUNTY_ORDER
    .map(code => {
      const r = rows.find(d=>d.county===code && d.period===latest);
      return r ? { code, name: COUNTY_NAMES[code]||code, value: r.value } : null;
    })
    .filter(Boolean)
    .sort((a,b) => b.value - a.value);

  const natAvgPrev = prev ? avg(rows.filter(d=>d.period===prev).map(d=>d.value)) : null;
  const natAvgNow  = avg(data.map(d=>d.value));
  const trendNote  = natAvgPrev && natAvgNow
    ? ` · trend: ${sign(natAvgNow-natAvgPrev)}${(natAvgNow-natAvgPrev).toFixed(1)} pp jämfört med ${prev}`
    : '';

  buildHBars('alcoholBarList', data, {
    colorFrom, colorTo, valueSuffix: ' %', baseline: 0,
    noteEl: 'alcoholNote',
    noteText: `Källa: Folkhälsomyndigheten · ${latest}${trendNote}`,
  });
}

/* ─── Sex toggles ────────────────────────────────────────────── */
function wireSexToggle(id, onChange) {
  document.getElementById(id).querySelectorAll('.sex-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.getElementById(id).querySelectorAll('.sex-btn').forEach(b=>b.classList.remove('active'));
      btn.classList.add('active');
      onChange(btn.dataset.sex);
    });
  });
}

wireSexToggle('lifeSeriesToggle', sex => { lifeSeriesSex = sex; buildLifeChart(); });
wireSexToggle('lifeBarToggle',    sex => { lifeBarSex    = sex; buildLifeBars(sex); });
wireSexToggle('smokeToggle',      sex => { smokeSex      = sex; buildSmokeBars(sex); });
wireSexToggle('alcoholToggle',    sex => { alcoholSex    = sex; buildAlcoholBars(sex); });

/* ─── Dödsorsaker ────────────────────────────────────────────── */
let deathsLineChart = null;
async function loadDeaths() {
  try {
    const r = await fetch('/api/v1/halsa/dodsorsaker');
    if (!r.ok) throw new Error(r.status);
    const rows = (await r.json()).data || [];

    // Group by diagnos, sort years.
    const diagnosList = ['99','I00-I99','C00-D48','Alk','Spec1'];
    const diagnosLabels = {
      '99':'Alla dödsorsaker','I00-I99':'Hjärt-/kärlsjukdomar',
      'C00-D48':'Cancer','Alk':'Alkohol','Spec1':'Narkotika/läkemedel'
    };
    const colors = ['#00bfa5','#ef5350','#ab47bc','#ffa726','#42a5f5'];
    const years = [...new Set(rows.map(d=>d.year))].sort();

    const datasets = diagnosList.map((id,i) => ({
      label: diagnosLabels[id],
      data: years.map(y => {
        const row = rows.find(r=>r.diagnos_id===id && r.year===y);
        return row ? row.total : null;
      }),
      borderColor: colors[i], backgroundColor: colors[i]+'22',
      tension: 0.3, pointRadius: 3, fill: false,
    }));

    if (deathsLineChart) deathsLineChart.destroy();
    deathsLineChart = new Chart(document.getElementById('deathsLineChart'), {
      type:'line',
      data:{ labels: years, datasets },
      options:{
        responsive:true,
        plugins:{ legend:{ position:'bottom', labels:{ color:'#ccc', font:{size:11} } } },
        scales:{
          x:{ ticks:{color:'#aaa'}, grid:{color:'#333'} },
          y:{ ticks:{color:'#aaa'}, grid:{color:'#333'},
              title:{display:true,text:'Dödsfall',color:'#aaa'} }
        }
      }
    });

    // Latest year table — excluding row 99 (all causes) to avoid clutter.
    const latestYear = years[years.length-1];
    const latestRows = rows.filter(r=>r.year===latestYear && r.diagnos_id!=='99')
      .sort((a,b)=>b.total-a.total);
    const tbody = document.getElementById('deathsLatestTbody');
    tbody.innerHTML = latestRows.map(r=>`
      <tr><td>${diagnosLabels[r.diagnos_id]||r.diagnos_id}</td>
          <td style="text-align:right">${r.total.toLocaleString('sv-SE')}</td></tr>
    `).join('');
    document.getElementById('deathsSub').textContent = `riket · alla åldrar · senaste: ${latestYear}`;
  } catch(e) { console.error('deaths fetch failed', e); }
}

/* ─── Tandhälsa ──────────────────────────────────────────────── */
let dentalBarChart = null;
async function loadDental() {
  try {
    const r = await fetch('/api/v1/halsa/tandhalsa');
    if (!r.ok) throw new Error(r.status);
    const rows = (await r.json()).data || [];

    const latestYear = Math.max(...rows.map(d=>d.year));
    // County-level rows: region_name contains "län" (counties) or is a known county name.
    const countyRows = rows.filter(d=>d.year===latestYear && d.county_code
        && (d.region_name.includes('län') || d.region_name === 'Riket' === false))
      .filter(d=>d.region_name.includes('län'))
      .sort((a,b)=>b.value-a.value);

    if (dentalBarChart) dentalBarChart.destroy();
    dentalBarChart = new Chart(document.getElementById('dentalBarChart'), {
      type:'bar',
      data:{
        labels: countyRows.map(d=>d.region_name.replace(' län','')),
        datasets:[{
          label:'Tandhälsobesök %',
          data: countyRows.map(d=>d.value),
          backgroundColor: countyRows.map((_,i)=>`hsl(${170+i*8},60%,45%)88`),
          borderColor:'#00bfa5',
          borderWidth:1
        }]
      },
      options:{
        indexAxis:'y', responsive:true,
        plugins:{ legend:{display:false} },
        scales:{
          x:{ min:40, ticks:{color:'#aaa'}, grid:{color:'#333'},
              title:{display:true,text:'%',color:'#aaa'} },
          y:{ ticks:{color:'#aaa',font:{size:10}}, grid:{color:'#222'} }
        }
      }
    });

    // Municipality rows: no "län" in name, have a county_code.
    const muniRows = rows.filter(d=>d.year===latestYear && d.county_code
        && !d.region_name.includes('län') && d.region_name !== 'Riket')
      .sort((a,b)=>b.value-a.value);

    // Top 10.
    document.getElementById('dentalTopTbody').innerHTML = muniRows.slice(0,10).map(r=>`
      <tr><td>${r.region_name}</td><td style="text-align:right;color:#2dd4a0">${r.value?.toFixed(1)||'–'} %</td></tr>
    `).join('');
    // Worst 10.
    document.getElementById('dentalWorstTbody').innerHTML = muniRows.slice(-10).reverse().map(r=>`
      <tr><td>${r.region_name}</td><td style="text-align:right;color:#ef5350">${r.value?.toFixed(1)||'–'} %</td></tr>
    `).join('');
    document.getElementById('dentalSub').textContent = `${latestYear} · ${countyRows.length} landsting · ${muniRows.length} kommuner`;
  } catch(e) { console.error('dental fetch failed', e); }
}

loadData();
loadDeaths();
loadDental();
