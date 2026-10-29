'use strict';

Chart.defaults.color = '#6b82a0';
Chart.defaults.borderColor = '#1b2940';
Chart.defaults.font.family = "'DM Mono', monospace";

const AREA_COLORS = { SE1: '#38b6ff', SE2: '#2dd4a0', SE3: '#e3b341', SE4: '#bc8cff' };
const AREA_NAMES  = { SE1: 'Norra Sverige', SE2: 'Norra Mellansverige', SE3: 'Stockholm/Söder Mellansv.', SE4: 'Södra Sverige (Malmö)' };

function esc(s) { return String(s??'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }

let allElpris = [];
let elChart = null;
let activeArea = 'SE3';

// ── Load ─────────────────────────────────────────────────────────────────────
Promise.allSettled([
  fetch('/api/v1/riksbank').then(r => r.json()),
  fetch('/api/v1/elpris').then(r => r.json()),
]).then(([rbRes, elRes]) => {
  if (rbRes.status === 'fulfilled') buildRiksbank(rbRes.value.data || []);
  if (elRes.status === 'fulfilled') {
    allElpris = elRes.value.data || [];
    buildAreaCards(allElpris);
    buildElChart(activeArea);
  }
});

// ── Riksbank ─────────────────────────────────────────────────────────────────
function buildRiksbank(data) {
  const byS = {};
  data.forEach(d => { (byS[d.series_id] = byS[d.series_id] || []).push(d); });

  const repoData  = (byS['policy_rate'] || []).sort((a,b) => a.date.localeCompare(b.date));
  const eurData   = (byS['sek_eur']     || []).sort((a,b) => a.date.localeCompare(b.date));
  const usdData   = (byS['sek_usd']     || []).sort((a,b) => a.date.localeCompare(b.date));

  if (repoData.length) {
    const last = repoData[repoData.length - 1];
    document.getElementById('s-repo').textContent = last.value.toFixed(2) + ' %';
    document.getElementById('s-repo-date').textContent = last.date;
  }
  if (eurData.length) {
    const last = eurData[eurData.length - 1];
    document.getElementById('s-eur').textContent = last.value.toFixed(4);
    document.getElementById('s-eur-date').textContent = last.date;
  }
  if (usdData.length) {
    const last = usdData[usdData.length - 1];
    document.getElementById('s-usd').textContent = last.value.toFixed(4);
    document.getElementById('s-usd-date').textContent = last.date;
  }

  // Sample monthly for performance (6,000+ daily points)
  function sampleMonthly(arr) {
    const seen = new Set();
    return arr.filter(d => { const m = d.date.slice(0,7); if (seen.has(m)) return false; seen.add(m); return true; });
  }

  buildLineChart('chart-repo', sampleMonthly(repoData), '#e3b341', '%', 'Styrränta', 0);
  buildLineChart('chart-eur', sampleMonthly(eurData), '#38b6ff', 'SEK', 'EUR/SEK', 2);
  buildLineChart('chart-usd', sampleMonthly(usdData), '#2dd4a0', 'SEK', 'USD/SEK', 2);
}

function buildLineChart(id, data, color, unit, label, decimals) {
  if (!data.length) return;
  new Chart(document.getElementById(id), {
    type: 'line',
    data: {
      labels: data.map(d => d.date.slice(0,7)),
      datasets: [{
        label,
        data: data.map(d => d.value),
        borderColor: color,
        backgroundColor: color + '14',
        borderWidth: 1.8,
        pointRadius: 0,
        pointHoverRadius: 4,
        tension: .2,
        fill: true,
      }]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
          callbacks: { label: c => ' ' + c.parsed.y.toFixed(decimals) + ' ' + unit }
        }
      },
      scales: {
        x: { ticks: { color: '#3d5270', maxTicksLimit: 10 }, grid: { color: 'rgba(27,41,64,.4)' } },
        y: { ticks: { color: '#3d5270', callback: v => v.toFixed(decimals) + ' ' + unit }, grid: { color: 'rgba(27,41,64,.4)' } }
      }
    }
  });
}

// ── Electricity ───────────────────────────────────────────────────────────────
// Returns the slot whose 15-min window contains the current moment.
function currentSlotPrice(area) {
  const nowMs = Date.now();
  return allElpris.find(d => {
    if (d.area !== area) return false;
    const tStart = new Date(d.time_start).getTime();
    return nowMs >= tStart && nowMs < tStart + 15 * 60 * 1000;
  }) || null;
}

// Swedish calendar date as "YYYY-MM-DD" (handles UTC offset in summer/winter)
function todaySE() {
  return new Date().toLocaleDateString('sv-SE', { timeZone: 'Europe/Stockholm' });
}

function ore(sekPerKwh) { return (sekPerKwh * 100).toFixed(1); }

function buildAreaCards(data) {
  const areas = ['SE1','SE2','SE3','SE4'];
  const prices = areas.map(a => {
    const r = currentSlotPrice(a);
    return r ? r.sek_per_kwh * 100 : null;
  });
  const maxP = Math.max(...prices.filter(p => p != null));

  areas.forEach((a, i) => {
    const p = prices[i];
    const pEl = document.getElementById('p-' + a.toLowerCase());
    const bEl = document.getElementById('b-' + a.toLowerCase());
    if (p != null) {
      pEl.textContent = p.toFixed(1);
      bEl.style.width = (maxP > 0 ? p / maxP * 100 : 0) + '%';
    }
  });

  // Hero SE3
  const se3now = currentSlotPrice('SE3');
  if (se3now) {
    document.getElementById('s-el').textContent = ore(se3now.sek_per_kwh) + ' öre';
  }
}

function buildElChart(area) {
  const seToday = todaySE();
  const nowMs = Date.now();
  const todayData = allElpris
    .filter(d => {
      if (d.area !== area) return false;
      const localDate = new Date(d.time_start).toLocaleDateString('sv-SE', { timeZone: 'Europe/Stockholm' });
      return localDate === seToday;
    })
    .sort((a,b) => a.time_start.localeCompare(b.time_start));

  if (!todayData.length) return;
  const prices = todayData.map(d => +(d.sek_per_kwh * 100).toFixed(2));
  const labels = todayData.map(d => new Date(d.time_start).toLocaleTimeString('sv-SE', { hour:'2-digit', minute:'2-digit', timeZone:'Europe/Stockholm' }));
  const colors = todayData.map(d => {
    const tStart = new Date(d.time_start).getTime();
    return (nowMs >= tStart && nowMs < tStart + 15 * 60 * 1000) ? AREA_COLORS[area] : AREA_COLORS[area] + '80';
  });

  if (elChart) elChart.destroy();
  elChart = new Chart(document.getElementById('chart-el'), {
    type: 'bar',
    data: {
      labels,
      datasets: [{
        label: 'öre/kWh',
        data: prices,
        backgroundColor: colors,
        borderRadius: 4,
        borderWidth: 0,
      }]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
          callbacks: { label: c => ' ' + c.parsed.y.toFixed(1) + ' öre/kWh' }
        }
      },
      scales: {
        x: { grid: { display: false }, ticks: { color: '#3d5270', maxTicksLimit: 12 } },
        y: { beginAtZero: true, ticks: { color: '#3d5270', callback: v => v + ' öre' }, grid: { color: 'rgba(27,41,64,.4)' } }
      }
    }
  });

  document.getElementById('el-note').textContent =
    'Spotpris per kvart (15 min) idag · ' + AREA_NAMES[area] + ' · Nord Pool via elprisetjustnu.se · exkl. moms';
}

// ── Area toggle ───────────────────────────────────────────────────────────────
document.getElementById('area-toggle').querySelectorAll('.area-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('#area-toggle .area-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    activeArea = btn.dataset.a;
    buildElChart(activeArea);
  });
});
