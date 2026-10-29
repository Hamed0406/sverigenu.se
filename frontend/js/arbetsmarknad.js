'use strict';

Chart.defaults.color = '#6b82a0';
Chart.defaults.borderColor = '#1b2940';
Chart.defaults.font.family = "'DM Mono', monospace";

const fmt = new Intl.NumberFormat('sv-SE');

function esc(s) { return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }

let allJobs = [];
let unempByCounty = {}; // county_code → avg %
let unempByKommun = []; // all municipality rows

Promise.allSettled([
  fetch('/api/v1/arbetsmarknad/jobs').then(r => r.json()),
  fetch('/api/v1/arbetsmarknad/fields').then(r => r.json()),
  fetch('/api/v1/kolada?kpi=N03920').then(r => r.json()),
]).then(([jobsRes, fieldsRes, unempRes]) => {
  if (jobsRes.status === 'fulfilled') {
    allJobs = jobsRes.value.data || [];
    buildHeroJobs(allJobs);
    buildJobsChart(allJobs);
  }
  if (fieldsRes.status === 'fulfilled') {
    const fields = fieldsRes.value.data || [];
    buildFieldsChart(fields);
    buildHeroField(fields);
  }
  if (unempRes.status === 'fulfilled') {
    const rows = unempRes.value.data || [];
    buildUnemployment(rows);
  }
});

// ── Hero ──────────────────────────────────────────────────────────────────────
function buildHeroJobs(jobs) {
  const total = jobs.reduce((s, j) => s + j.total_ads, 0);
  document.getElementById('s-total').textContent = fmt.format(total);
  if (jobs.length) {
    const top = jobs[0]; // already sorted desc
    document.getElementById('s-top-county').textContent = fmt.format(top.total_ads);
    document.getElementById('s-top-county-name').textContent = top.name;
  }
}

function buildHeroField(fields) {
  if (!fields.length) return;
  const top = fields[0];
  document.getElementById('s-top-field').textContent = top.term;
  document.getElementById('s-top-field-count').textContent = fmt.format(top.count) + ' annonser';
}

// ── Jobs per county chart ─────────────────────────────────────────────────────
function buildJobsChart(jobs) {
  if (!jobs.length) return;
  const labels = jobs.map(j => j.name);
  const data   = jobs.map(j => j.total_ads);
  const max    = data[0];
  const colors = data.map(v => {
    const r = v / max;
    return r > .6 ? '#2dd4a0' : r > .3 ? '#2dd4a080' : '#2dd4a040';
  });

  new Chart(document.getElementById('chart-jobs'), {
    type: 'bar',
    data: { labels, datasets: [{ label: 'Annonser', data, backgroundColor: colors, borderRadius: 4, borderWidth: 0 }] },
    options: {
      indexAxis: 'y',
      responsive: true, maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: { backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
          callbacks: { label: c => ' ' + fmt.format(c.parsed.x) + ' annonser' } }
      },
      scales: {
        x: { beginAtZero: true, ticks: { color: '#3d5270', callback: v => fmt.format(v) }, grid: { color: 'rgba(27,41,64,.4)' } },
        y: { ticks: { color: '#6b82a0', font: { size: 10 } }, grid: { display: false } }
      }
    }
  });
}

// ── Occupation fields doughnut ────────────────────────────────────────────────
function buildFieldsChart(fields) {
  if (!fields.length) return;
  const top = fields.slice(0, 8);
  const colors = ['#2dd4a0','#38b6ff','#e3b341','#bc8cff','#3fb950','#ff7b72','#f78166','#daaa3f'];
  new Chart(document.getElementById('chart-fields'), {
    type: 'doughnut',
    data: {
      labels: top.map(f => f.term),
      datasets: [{ data: top.map(f => f.count), backgroundColor: colors, borderColor: '#0f1726', borderWidth: 2 }]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      cutout: '62%',
      plugins: {
        legend: { position: 'right', labels: { color: '#6b82a0', font: { size: 10 }, boxWidth: 12, padding: 10 } },
        tooltip: { backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
          callbacks: { label: c => ' ' + fmt.format(c.parsed) + ' annonser' } }
      }
    }
  });
}

// ── Unemployment ──────────────────────────────────────────────────────────────
function buildUnemployment(rows) {
  // Find latest period per municipality
  const latest = {};
  rows.forEach(r => {
    if (r.value == null) return;
    const key = r.municipality_id || r.municipality;
    if (!latest[key] || r.period > latest[key].period) latest[key] = r;
  });
  const all = Object.values(latest);

  // County averages
  const byCo = {};
  all.forEach(r => {
    if (!byCo[r.county_code]) byCo[r.county_code] = [];
    byCo[r.county_code].push(r.value);
  });
  Object.keys(byCo).forEach(c => {
    const arr = byCo[c];
    unempByCounty[c] = arr.reduce((s, v) => s + v, 0) / arr.length;
  });

  // National average
  if (all.length) {
    const avg = all.reduce((s, r) => s + r.value, 0) / all.length;
    document.getElementById('s-unemp').textContent = avg.toFixed(1) + ' %';
  }

  // County table with jobs + unemployment
  buildCountyTable();

  // Top/bottom municipalities
  const sorted = [...all].sort((a, b) => b.value - a.value);
  const high = sorted.slice(0, 10);
  const low  = sorted.slice(-10).reverse();
  const tbody = (id, arr) => {
    const el = document.getElementById(id);
    el.innerHTML = arr.map((r, i) => `
      <tr>
        <td class="td-rank">${i+1}</td>
        <td class="td-name">${esc(r.municipality)}</td>
        <td class="td-val">${Number(r.value).toFixed(1)} %</td>
      </tr>`).join('');
  };
  tbody('high-unemp', high);
  tbody('low-unemp', low);
}

function buildCountyTable() {
  const tbody = document.getElementById('county-tbody');
  const maxJobs = allJobs.length ? allJobs[0].total_ads : 1;
  tbody.innerHTML = allJobs.map((j, i) => {
    const unemp = unempByCounty[j.county_code];
    const unempStr = unemp != null ? unemp.toFixed(1) + ' %' : '—';
    const pct = (j.total_ads / maxJobs * 100).toFixed(1);
    return `<tr>
      <td style="color:var(--dim);font-family:'DM Mono',monospace;font-size:.72rem">${i+1}</td>
      <td class="td-name">${esc(j.name)}</td>
      <td class="td-jobs">${fmt.format(j.total_ads)}</td>
      <td class="td-unemp">${unempStr}</td>
      <td class="ct-bar-cell"><div class="ct-bar"><div class="ct-fill" style="width:${pct}%"></div></div></td>
    </tr>`;
  }).join('');
}
