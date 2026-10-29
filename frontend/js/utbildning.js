'use strict';

function esc(s) { return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }
function fmt(v) { return v == null ? '—' : Math.round(v).toLocaleString('sv-SE'); }
function fmtDec(v, d=1) { return v == null ? '—' : Number(v).toFixed(d).replace('.',','); }

let allData = [];
let salsaMap = {};   // municipality_code → salsa row
let sortCol = 'merit';
let sortDir = -1;
let tableFilter = '';
let highlightCode = null;
let nationalMeritAvg = 0;
let nationalTeacherAvg = 0;

// ── Load data ─────────────────────────────────────────────────────────────────
Promise.all([
  fetch('/api/v1/utbildning/schools').then(r => r.json()),
  fetch('/api/v1/kolada?kpi=N15504').then(r => r.json()),
  fetch('/api/v1/kolada?kpi=N15033').then(r => r.json()),
  fetch('/api/v1/utbildning/salsa').then(r => r.json()).catch(() => ({data:[]})),
  fetch('/api/v1/utbildning/national').then(r => r.json()).catch(() => ({})),
]).then(([schoolsRes, meritRes, teacherRes, salsaRes, natRes]) => {
  // National stats
  buildNational(natRes);

  // SALSA map
  (salsaRes.data || []).forEach(s => { salsaMap[s.municipality_code] = s; });

  const schoolMap = {}, meritMap = {}, teacherMap = {};
  (schoolsRes.data || []).forEach(s => { schoolMap[s.municipality_code] = s; });
  (meritRes.data  || []).forEach(m => { meritMap[m.municipality]  = m; });
  (teacherRes.data || []).forEach(t => { teacherMap[t.municipality] = t; });

  const codes = new Set([
    ...Object.keys(schoolMap),
    ...Object.keys(meritMap),
    ...Object.keys(teacherMap),
  ]);

  allData = [...codes].map(code => {
    const s = schoolMap[code]  || {};
    const m = meritMap[code]   || {};
    const t = teacherMap[code] || {};
    const sal = salsaMap[code] || {};
    return { code, name: m.name || t.name || code,
      merit: m.value ?? null, teacher: t.value ?? null,
      schoolCount: s.school_count ?? null, period: m.period ?? null,
      deviation: sal.avg_deviation ?? null,
      reqReached: sal.req_reached_actual ?? null,
    };
  }).filter(d => d.merit != null || d.teacher != null);

  // sort by merit for global rank assignment
  const ranked = [...allData].filter(d => d.merit != null).sort((a,b) => b.merit - a.merit);
  const rankMap = {};
  ranked.forEach((d, i) => { rankMap[d.code] = i + 1; });
  allData.forEach(d => { d.rank = rankMap[d.code] ?? null; });

  const withMerit = allData.filter(d => d.merit != null);
  nationalMeritAvg = withMerit.reduce((s,d) => s+d.merit, 0) / withMerit.length;
  const withTeacher = allData.filter(d => d.teacher != null);
  nationalTeacherAvg = withTeacher.reduce((s,d) => s+d.teacher, 0) / withTeacher.length;

  buildHero();
  buildScale();
  buildSalsa();
  buildTopBottom();
  buildTeacherRanking();
  buildTable();
  wireSearch();
});

// ── National stats ────────────────────────────────────────────────────────────
function buildNational(nat) {
  if (!nat || !nat.period) return;
  document.getElementById('natPeriod').textContent = 'Skolverket · läsår ' + nat.period;
  if (nat.avg_merit_9th != null)          document.getElementById('natMerit').textContent   = fmtDec(nat.avg_merit_9th, 0);
  if (nat.certified_teachers_pct != null) document.getElementById('natCert').textContent    = fmtDec(nat.certified_teachers_pct, 1) + ' %';
  if (nat.ratio_passed_9th != null)       document.getElementById('natPassed9').textContent = fmtDec(nat.ratio_passed_9th, 1) + ' %';
  if (nat.ratio_passed_6th != null)       document.getElementById('natPassed6').textContent = fmtDec(nat.ratio_passed_6th, 1) + ' %';

  // Nationella prov comparison grid
  const subjects = [
    { label: 'Svenska', key6: 'test_sve_6', key9: 'test_sve_9' },
    { label: 'Engelska', key6: 'test_eng_6', key9: 'test_eng_9' },
    { label: 'Matematik', key6: 'test_ma_6', key9: 'test_ma_9' },
  ];
  const grid = document.getElementById('natTestGrid');
  const maxScore = 20;
  grid.innerHTML = subjects.map(s => {
    const v6 = nat[s.key6], v9 = nat[s.key9];
    const pct6 = v6 != null ? Math.round((v6 / maxScore) * 100) : 0;
    const pct9 = v9 != null ? Math.round((v9 / maxScore) * 100) : 0;
    return `<div class="nat-test-card">
      <div class="nat-test-subject">${s.label}</div>
      <div class="nat-test-row">
        <div class="nat-test-grade">Åk 6</div>
        <div class="nat-test-bar-wrap"><div class="nat-test-bar ak6" style="width:0%" data-w="${pct6}%"></div></div>
        <div class="nat-test-val" style="color:var(--gold)">${v6 != null ? fmtDec(v6,1) : '—'}</div>
      </div>
      <div class="nat-test-row">
        <div class="nat-test-grade">Åk 9</div>
        <div class="nat-test-bar-wrap"><div class="nat-test-bar ak9" style="width:0%" data-w="${pct9}%"></div></div>
        <div class="nat-test-val" style="color:var(--edu)">${v9 != null ? fmtDec(v9,1) : '—'}</div>
      </div>
      <div style="font-size:.68rem; color:var(--dim); margin-top:10px">av max ${maxScore} poäng</div>
    </div>`;
  }).join('');

  // Animate bars after a tick
  setTimeout(() => {
    grid.querySelectorAll('.nat-test-bar[data-w]').forEach(el => {
      el.style.width = el.getAttribute('data-w');
    });
  }, 60);

  // Set test period from åk 6 data (2024/25, one year behind åk 9)
  const testPeriod = nat.test_sve_6 != null ? '2024/25' : nat.period;
  document.getElementById('natTestPeriod').textContent = 'Skolverket · nationella prov · medelpoäng per ämne (skala 0–20) · ' + testPeriod;
}

// ── Hero ──────────────────────────────────────────────────────────────────────
function buildHero() {
  document.getElementById('heroMerit').innerHTML = fmtDec(nationalMeritAvg, 0) + '<span class="of"> / 340</span>';
  document.getElementById('heroTeacher').textContent = fmtDec(nationalTeacherAvg);
  const withSchools = allData.filter(d => d.schoolCount != null);
  document.getElementById('heroSchools').textContent = fmt(withSchools.reduce((s,d) => s+d.schoolCount, 0));
  const year = allData.find(d => d.period)?.period;
  if (year) {
    document.getElementById('heroYear').textContent = year + '/' + (String(year+1).slice(-2));
    document.getElementById('rankYear').textContent = 'läsår ' + year + '/' + (String(year+1).slice(-2));
  }
}

// ── Scale visual ──────────────────────────────────────────────────────────────
function buildScale() {
  const MAX = 340;
  const withMerit = allData.filter(d => d.merit != null).sort((a,b) => a.merit - b.merit);
  const minVal = withMerit[0]?.merit ?? 0;
  const maxVal = withMerit[withMerit.length-1]?.merit ?? 340;

  // Fill only covers the populated range
  const fillPct = (maxVal / MAX) * 100;
  document.getElementById('scaleFill').style.width = fillPct + '%';

  const track = document.getElementById('scaleTrack');
  const markers = [
    { val: minVal, label: 'Lägst ' + fmtDec(minVal,0), color: '#ff7b72', bg: 'rgba(255,123,114,.12)' },
    { val: nationalMeritAvg, label: 'Rikssnitt ' + fmtDec(nationalMeritAvg,0), color: '#7c9eff', bg: 'rgba(124,158,255,.14)' },
    { val: maxVal, label: 'Högst ' + fmtDec(maxVal,0), color: '#3fb950', bg: 'rgba(63,185,80,.12)' },
  ];
  markers.forEach(m => {
    const pct = (m.val / MAX) * 100;
    const el = document.createElement('div');
    el.className = 'scale-marker';
    el.style.left = pct + '%';
    el.innerHTML = `<div class="scale-marker-line" style="background:${m.color}"></div>
      <div class="scale-marker-label" style="color:${m.color};background:${m.bg}">${m.label}</div>`;
    track.appendChild(el);
  });
}

// ── SALSA value-add ranking ───────────────────────────────────────────────────
function buildSalsa() {
  const withDev = [...allData].filter(d => d.deviation != null);
  if (!withDev.length) return;

  document.getElementById('salsaSub').textContent = 'SALSA · Skolverket · ' + withDev.length + ' kommuner';

  const sorted = [...withDev].sort((a, b) => b.deviation - a.deviation);
  const absMax = Math.max(...withDev.map(d => Math.abs(d.deviation)));

  function renderSalsa(id, items, isTop) {
    const el = document.getElementById(id);
    el.innerHTML = '';
    items.forEach((d, i) => {
      const pct = Math.round((Math.abs(d.deviation) / absMax) * 100);
      const isPos = d.deviation >= 0;
      const color = isPos ? 'hbar-fill good' : 'hbar-fill bad';
      const sign = isPos ? '+' : '';
      const row = document.createElement('div');
      row.className = 'hbar-row';
      row.innerHTML =
        `<div class="hbar-rank">${i+1}</div>
         <div class="hbar-right">
           <div class="hbar-name">${esc(d.name)}</div>
           <div class="hbar-track"><div class="${color}" style="width:0%"></div></div>
         </div>
         <div class="hbar-val">${sign}${fmtDec(d.deviation,0)} p</div>`;
      el.appendChild(row);
      setTimeout(() => {
        row.querySelector('.hbar-fill').style.width = pct + '%';
        row.classList.add('in');
      }, 30 + i * 30);
    });
  }

  renderSalsa('salsaTopList',    sorted.slice(0, 10), true);
  renderSalsa('salsaBottomList', [...sorted].reverse().slice(0, 10), false);
}

// ── Top / Bottom bar lists ────────────────────────────────────────────────────
function renderHbarList(id, items, maxVal, colorFn) {
  const el = document.getElementById(id);
  el.innerHTML = '';
  items.forEach((d, i) => {
    const pct = Math.round((d.merit / maxVal) * 100);
    const row = document.createElement('div');
    row.className = 'hbar-row';
    const fill = colorFn ? colorFn(d) : 'hbar-fill';
    row.innerHTML =
      `<div class="hbar-rank">${i+1}</div>
       <div class="hbar-right">
         <div class="hbar-name">${esc(d.name)}</div>
         <div class="hbar-track"><div class="${fill}" style="width:0%"></div></div>
       </div>
       <div class="hbar-val">${fmtDec(d.merit,0)} p</div>`;
    el.appendChild(row);
    setTimeout(() => {
      row.querySelector('.hbar-fill, .hbar-fill.good, .hbar-fill.bad').style.width = pct + '%';
      row.classList.add('in');
    }, 30 + i * 30);
  });
}

function buildTopBottom() {
  const sorted = [...allData].filter(d => d.merit != null).sort((a,b) => b.merit - a.merit);
  const maxVal = sorted[0]?.merit ?? 1;
  renderHbarList('topList',    sorted.slice(0, 10), maxVal, () => 'hbar-fill good');
  renderHbarList('bottomList', [...sorted].reverse().slice(0, 10), maxVal, () => 'hbar-fill bad');
}

// ── Teacher ranking ───────────────────────────────────────────────────────────
function buildTeacherRanking() {
  const sorted = [...allData].filter(d => d.teacher != null).sort((a,b) => a.teacher - b.teacher);
  const maxTeacher = sorted[sorted.length-1]?.teacher ?? 1;

  function renderTeacher(id, items) {
    const el = document.getElementById(id);
    el.innerHTML = '';
    items.forEach((d, i) => {
      const pct = Math.round((d.teacher / maxTeacher) * 100);
      const row = document.createElement('div');
      row.className = 'hbar-row';
      row.innerHTML =
        `<div class="hbar-rank">${i+1}</div>
         <div class="hbar-right">
           <div class="hbar-name">${esc(d.name)}</div>
           <div class="hbar-track"><div class="hbar-fill" style="width:0%"></div></div>
         </div>
         <div class="hbar-val">${fmtDec(d.teacher)} elever</div>`;
      el.appendChild(row);
      setTimeout(() => {
        row.querySelector('.hbar-fill').style.width = pct + '%';
        row.classList.add('in');
      }, 30 + i * 30);
    });
  }

  renderTeacher('teacherBestList',  sorted.slice(0, 10));
  renderTeacher('teacherWorstList', [...sorted].reverse().slice(0, 10));
}

// ── Full table ────────────────────────────────────────────────────────────────
function buildTable() {
  let rows = [...allData].filter(d => d.merit != null || d.teacher != null);

  // Filter by name
  if (tableFilter) {
    const q = tableFilter.toLowerCase();
    rows = rows.filter(d => d.name.toLowerCase().includes(q));
  }

  // Sort
  rows.sort((a, b) => {
    const av = a[sortCol] ?? -Infinity;
    const bv = b[sortCol] ?? -Infinity;
    if (sortCol === 'name') return sortDir * a.name.localeCompare(b.name, 'sv');
    return sortDir * (bv - av);
  });

  document.getElementById('tableSub').textContent = rows.length + ' kommuner';

  document.getElementById('muniBody').innerHTML = rows.map((d, i) => {
    const hl = d.code === highlightCode ? ' hl' : '';
    let devCell = '—';
    if (d.deviation != null) {
      const sign = d.deviation >= 0 ? '+' : '';
      const cls = d.deviation > 3 ? 'pos' : d.deviation < -3 ? 'neg' : 'neu';
      devCell = `<span class="dev-chip ${cls}">${sign}${fmtDec(d.deviation,0)}</span>`;
    }
    return `<tr class="${hl}">
      <td class="rank-cell">${d.rank ?? '—'}</td>
      <td class="mname">${esc(d.name)}</td>
      <td>${d.merit != null ? fmtDec(d.merit,0) : '—'}</td>
      <td>${devCell}</td>
      <td>${d.teacher != null ? fmtDec(d.teacher) : '—'}</td>
      <td>${d.schoolCount ?? '—'}</td>
    </tr>`;
  }).join('');
}

// ── Municipality search ───────────────────────────────────────────────────────
function wireSearch() {
  const input = document.getElementById('muniSearch');
  const clearBtn = document.getElementById('searchClear');
  const result = document.getElementById('muniResult');
  const noResult = document.getElementById('noResult');

  input.addEventListener('input', () => {
    const q = input.value.trim().toLowerCase();
    if (!q) { result.classList.remove('visible'); noResult.classList.remove('visible'); return; }

    const match = allData.find(d => d.name.toLowerCase() === q)
      || allData.find(d => d.name.toLowerCase().startsWith(q))
      || allData.find(d => d.name.toLowerCase().includes(q));

    if (!match) {
      result.classList.remove('visible');
      noResult.classList.add('visible');
      highlightCode = null;
      buildTable();
      return;
    }

    noResult.classList.remove('visible');
    result.classList.add('visible');
    highlightCode = match.code;

    // Populate result card
    const merit = match.merit;
    const diff = merit != null ? merit - nationalMeritAvg : null;
    const meritEl = document.getElementById('res-merit');
    meritEl.textContent = merit != null ? fmtDec(merit, 0) + ' p' : '—';
    meritEl.className = 'muni-stat-num' + (diff > 8 ? ' better' : diff < -8 ? ' worse' : '');
    if (diff != null) {
      document.getElementById('res-merit-sub').textContent =
        (diff >= 0 ? '+' : '') + fmtDec(diff, 0) + ' vs rikssnitt (' + fmtDec(nationalMeritAvg, 0) + ' p)';
    }

    document.getElementById('res-rank').textContent = match.rank ? '#' + match.rank + ' av 290' : '—';
    document.getElementById('res-teacher').textContent = match.teacher != null ? fmtDec(match.teacher) : '—';
    document.getElementById('res-teacher-sub').textContent = 'rikssnitt: ' + fmtDec(nationalTeacherAvg);
    document.getElementById('res-schools').textContent = match.schoolCount ?? '—';

    // Scroll to table and highlight
    tableFilter = '';
    document.getElementById('tableSearch').value = '';
    buildTable();
    setTimeout(() => {
      const hlRow = document.querySelector('#muniBody tr.hl');
      if (hlRow) hlRow.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 100);
  });

  clearBtn.addEventListener('click', () => {
    input.value = '';
    result.classList.remove('visible');
    noResult.classList.remove('visible');
    highlightCode = null;
    buildTable();
    input.focus();
  });
}

// ── Table controls ────────────────────────────────────────────────────────────
document.querySelectorAll('.sort-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.sort-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    sortCol = btn.dataset.col;
    sortDir = -1;
    buildTable();
  });
});

document.getElementById('tableSearch').addEventListener('input', e => {
  tableFilter = e.target.value.trim();
  buildTable();
});
