const WHO = { 1: 45, 2: 15, 5: 25, 8: 100 };
const PARAM_NAME = { 1: 'PM10', 2: 'PM2.5', 5: 'NO₂', 8: 'O₃' };
const PARAM_COLOR = { 1: '#f5a623', 2: '#e55a4e', 5: '#7b6cf6', 8: '#38b2d8' };
const WHO_LINE = 'rgba(255,80,80,.6)';
const CANVAS_ID = { 1: 'chartPm10', 2: 'chartPm25', 5: 'chartNo2', 8: 'chartO3' };

function median(arr) {
  if (!arr.length) return null;
  const s = [...arr].sort((a,b) => a-b);
  const m = Math.floor(s.length/2);
  return s.length % 2 ? s[m] : (s[m-1]+s[m])/2;
}

function fmt(v, decimals=1) {
  if (v == null) return '–';
  return v.toFixed(decimals).replace('.',',');
}

function qualityClass(v, param) {
  const lim = WHO[param];
  if (v == null) return '';
  if (v > lim) return 'badge-bad';
  if (v > lim * 0.7) return 'badge-warn';
  return 'badge-good';
}

function buildBarChart(canvasId, labels, values, color, whoLimit) {
  const ctx = document.getElementById(canvasId);
  if (!ctx) return;

  // WHO annotation line plugin (inline)
  const whoLinePlugin = {
    id: 'whoLine',
    afterDraw(chart) {
      const { ctx: c, scales: { x } } = chart;
      if (!x) return;
      const pxVal = x.getPixelForValue(whoLimit);
      if (pxVal < x.left || pxVal > x.right) return;
      c.save();
      c.beginPath();
      c.moveTo(pxVal, chart.chartArea.top);
      c.lineTo(pxVal, chart.chartArea.bottom);
      c.strokeStyle = WHO_LINE;
      c.lineWidth = 1.5;
      c.setLineDash([4, 3]);
      c.stroke();
      c.fillStyle = WHO_LINE;
      c.font = '10px DM Mono, monospace';
      c.fillText('WHO ' + whoLimit, pxVal + 3, chart.chartArea.top + 12);
      c.restore();
    }
  };

  const bgColors = values.map(v => v > whoLimit ? 'rgba(229,90,78,.7)' : color + 'bb');

  new Chart(ctx, {
    type: 'bar',
    plugins: [whoLinePlugin],
    data: {
      labels,
      datasets: [{
        data: values,
        backgroundColor: bgColors,
        borderRadius: 3,
        borderSkipped: false,
      }]
    },
    options: {
      indexAxis: 'y',
      responsive: true,
      maintainAspectRatio: false,
      plugins: { legend: { display: false }, tooltip: {
        callbacks: {
          label: ctx => ' ' + fmt(ctx.raw) + ' µg/m³'
        }
      }},
      scales: {
        x: {
          grid: { color: 'rgba(30,42,61,.8)' },
          ticks: { color: '#6b7a99', font: { size: 10, family: 'DM Mono' } },
          title: { display: false }
        },
        y: {
          grid: { display: false },
          ticks: { color: '#9ba8c0', font: { size: 10 }, maxRotation: 0 }
        }
      }
    }
  });
}

function buildStationTable(stations) {
  if (!stations.length) {
    document.getElementById('station-table-wrap').innerHTML = '<p class="empty">Inga stationsdata tillgängliga just nu.</p>';
    return;
  }

  // Sort by PM2.5 value desc, then name
  const sorted = [...stations].sort((a,b) => {
    if (b.value == null && a.value == null) return a.name.localeCompare(b.name);
    if (b.value == null) return -1;
    if (a.value == null) return 1;
    return b.value - a.value;
  });

  const rows = sorted.map(s => {
    const cls = qualityClass(s.value, s.parameter);
    return `<tr>
      <td>${s.name}</td>
      <td style="color:var(--muted);font-family:'DM Mono',monospace;font-size:.75rem">${s.county || '–'}</td>
      <td>${PARAM_NAME[s.parameter] || s.param_name}</td>
      <td class="val-cell ${cls}">${fmt(s.value)}</td>
      <td style="color:var(--muted);font-size:.75rem">${s.measured_at ? new Date(s.measured_at).toLocaleTimeString('sv-SE',{hour:'2-digit',minute:'2-digit'}) : '–'}</td>
    </tr>`;
  }).join('');

  document.getElementById('station-table-wrap').innerHTML = `
    <table class="station-table">
      <thead><tr>
        <th>Län</th><th>Kod</th><th>Parameter</th><th style="text-align:right">µg/m³</th><th>Tid</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>`;
  document.getElementById('station-count-label').textContent = sorted.length + ' länsmätpunkter (PM2.5)';
}

async function load() {
  let data = [];
  try {
    const res = await fetch('/api/v1/luftkvalitet');
    const json = await res.json();
    data = json.data || [];
  } catch(e) {
    document.querySelector('.page').innerHTML += '<p class="empty">Kunde inte hämta data. Försök igen senare.</p>';
    return;
  }

  if (!data.length) {
    document.querySelector('.page').innerHTML += '<p class="empty">Inga luftkvalitetsdata tillgängliga just nu. Försök igen om en stund.</p>';
    return;
  }

  // Group by parameter
  const byParam = {};
  for (const r of data) {
    if (!byParam[r.parameter]) byParam[r.parameter] = [];
    byParam[r.parameter].push(r);
  }

  // ── Hero ──
  const pm25Vals = (byParam[2] || []).filter(r => r.value != null).map(r => r.value);
  const no2Vals  = (byParam[5] || []).filter(r => r.value != null).map(r => r.value);
  const stationsWithData = data.filter(r => r.value != null);
  const uniqueStations = new Set(stationsWithData.map(r => r.station_key)).size;

  const exceedCount = data.filter(r => r.value != null && r.value > (WHO[r.parameter] || Infinity)).length;

  document.getElementById('h-stations').textContent = uniqueStations || '–';

  const pm25med = median(pm25Vals);
  const no2med  = median(no2Vals);

  const hPm25 = document.getElementById('h-pm25');
  hPm25.textContent = fmt(pm25med);
  if (pm25med != null) hPm25.className = 'hero-val ' + (pm25med > 15 ? 'bad' : pm25med > 10 ? 'warn' : 'good');

  const hNo2 = document.getElementById('h-no2');
  hNo2.textContent = fmt(no2med);
  if (no2med != null) hNo2.className = 'hero-val ' + (no2med > 25 ? 'bad' : no2med > 17 ? 'warn' : 'good');

  const hEx = document.getElementById('h-exceed');
  hEx.textContent = exceedCount;
  hEx.className = 'hero-val ' + (exceedCount > 0 ? 'bad' : 'good');

  // ── Insights ──
  const pm25max = pm25Vals.length ? Math.max(...pm25Vals) : null;
  const pm25maxStation = pm25max != null ? (byParam[2]||[]).find(r => r.value === pm25max) : null;
  document.getElementById('ins-pm25').innerHTML = pm25Vals.length
    ? `Median <b>${fmt(pm25med)} µg/m³</b> (WHO: 15). Högst: <b>${pm25maxStation?.name || '–'}</b> med ${fmt(pm25max)} µg/m³.`
    : 'Ingen PM2.5-data tillgänglig just nu.';

  const no2max = no2Vals.length ? Math.max(...no2Vals) : null;
  const no2maxStation = no2max != null ? (byParam[5]||[]).find(r => r.value === no2max) : null;
  document.getElementById('ins-no2').innerHTML = no2Vals.length
    ? `Median <b>${fmt(no2med)} µg/m³</b> (WHO: 25). Högst: <b>${no2maxStation?.name || '–'}</b> med ${fmt(no2max)} µg/m³.`
    : 'Ingen NO₂-data tillgänglig just nu.';

  const o3Vals = (byParam[8] || []).filter(r => r.value != null).map(r => r.value);
  const o3med  = median(o3Vals);
  const o3max  = o3Vals.length ? Math.max(...o3Vals) : null;
  const o3maxStation = o3max != null ? (byParam[8]||[]).find(r => r.value === o3max) : null;
  document.getElementById('ins-o3').innerHTML = o3Vals.length
    ? `Median <b>${fmt(o3med)} µg/m³</b> (WHO: 100). Högst: <b>${o3maxStation?.name || '–'}</b> med ${fmt(o3max)} µg/m³.`
    : 'Ingen O₃-data tillgänglig just nu.';

  // ── Bar charts ──
  const CHART_PARAMS = [
    { id: 2, limit: 15  },
    { id: 5, limit: 25  },
    { id: 1, limit: 45  },
    { id: 8, limit: 100 },
  ];

  for (const { id, limit } of CHART_PARAMS) {
    const rows = (byParam[id] || [])
      .filter(r => r.value != null)
      .sort((a,b) => b.value - a.value)
      .slice(0, 30); // cap at 30 for readability
    if (!rows.length) continue;

    const labels = rows.map(r => r.name.replace(/\s*\(.*\)$/, ''));
    const values = rows.map(r => r.value);
    buildBarChart(CANVAS_ID[id], labels, values, PARAM_COLOR[id], limit);
  }

  // ── Station table (PM2.5 only for conciseness) ──
  buildStationTable(byParam[2] || data.filter(r => r.parameter === 2));
}

load();
