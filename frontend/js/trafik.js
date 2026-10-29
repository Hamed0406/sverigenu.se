'use strict';
const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};

function esc(s) { return String(s??'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }
function condClass(sev) { const n=parseInt(sev,10); if(n===1)return'cond-1'; if(n===2)return'cond-2'; if(n===3)return'cond-3'; if(n>=4)return'cond-4'; return'cond-other'; }
function fmtTime(iso) { if(!iso)return'—'; try{return new Date(iso).toLocaleString('sv-SE',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'});}catch{return'—';} }

Chart.defaults.color = '#6b82a0';
Chart.defaults.borderColor = '#1b2940';
Chart.defaults.font.family = "'DM Mono', monospace";

let allCond = [];
let chartSpeed = null, chartPie = null;

// ── Flow cards ──────────────────────────────────────────────────────────────
function speedColor(kmh) {
  if (!kmh) return '#3d5270';
  if (kmh >= 80) return '#3fb950';
  if (kmh >= 50) return '#e3b341';
  if (kmh >= 30) return '#f0883e';
  return '#ff7b72';
}

function renderFlow(flowData) {
  const grid = document.getElementById('flow-grid');
  if (!flowData.length) {
    grid.innerHTML = '<div class="empty">Trafikflödesdata är inte tillgänglig just nu.</div>';
    return;
  }

  // Update stat band
  const allSpeeds = flowData.filter(d=>d.avg_speed).map(d=>d.avg_speed);
  const overallSpeed = allSpeeds.length ? allSpeeds.reduce((a,b)=>a+b,0)/allSpeeds.length : 0;
  const totalSensors = flowData.reduce((a,d) => a + (d.measurements||0), 0);
  const topFlow = flowData[0];

  document.getElementById('s-flow').textContent = topFlow && topFlow.avg_flow_per_lane != null
    ? topFlow.avg_flow_per_lane.toLocaleString('sv-SE') : '—';
  document.getElementById('s-speed').textContent = overallSpeed ? overallSpeed.toFixed(1) + ' km/h' : '—';
  document.getElementById('s-sensors').textContent = totalSensors.toLocaleString('sv-SE');

  grid.innerHTML = flowData.map(d => {
    const name = COUNTY_NAMES[d.county] || d.county.toUpperCase();
    const spd = d.avg_speed ? d.avg_speed.toFixed(1) : '—';
    const flow = d.avg_flow_per_lane != null ? d.avg_flow_per_lane.toLocaleString('sv-SE') : '—';
    const pct = d.avg_speed ? Math.min(100, (d.avg_speed / 110) * 100) : 0;
    const col = speedColor(d.avg_speed);
    return `<div class="flow-card">
      <div class="fc-county">${esc(d.county.toUpperCase())} — Trafikflöde</div>
      <div class="fc-name"><a href="/lan/${esc(d.county)}" style="color:inherit;text-decoration:none;">${esc(name)}</a></div>
      <div class="fc-stats">
        <div>
          <div class="fc-stat-val" style="color:${col}">${esc(spd)}<span style="font-size:.75rem;color:var(--muted)"> km/h</span></div>
          <div class="fc-stat-label">Medelhastighet</div>
        </div>
        <div>
          <div class="fc-stat-val" style="color:var(--orange)">${esc(flow)}</div>
          <div class="fc-stat-label">Fordon/körfält/h</div>
        </div>
      </div>
      <div class="speed-bar"><div class="speed-fill" style="width:${pct}%;background:${col}"></div></div>
    </div>`;
  }).join('');
}

// ── Charts ───────────────────────────────────────────────────────────────────
function buildCondPie(data) {
  const condCounts = {};
  data.forEach(d => { condCounts[d.site_type] = (condCounts[d.site_type]||0)+1; });
  const condLabels = Object.keys(condCounts).sort((a,b) => condCounts[b]-condCounts[a]);
  const pieColors = condLabels.map(l => {
    if (l==='Normalt') return '#3fb950';
    if (l.includes('Delvis')) return '#e3b341';
    if (l.includes('Hal')) return '#f0883e';
    if (l.includes('Mycket')) return '#ff7b72';
    return '#38b6ff';
  });

  if (chartPie) chartPie.destroy();
  chartPie = new Chart(document.getElementById('chart-pie'), {
    type: 'doughnut',
    data: { labels: condLabels, datasets: [{ data: condLabels.map(l=>condCounts[l]), backgroundColor: pieColors, borderWidth: 2, borderColor: '#0f1726' }] },
    options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'bottom', labels: { boxWidth: 10, padding: 12 } } } }
  });
}

function buildSpeedChart(flowData) {
  if (!flowData || !flowData.length) return;
  const labels = flowData.map(d => COUNTY_NAMES[d.county] || d.county.toUpperCase());
  const speeds = flowData.map(d => d.avg_speed || 0);
  const colors = speeds.map(s => speedColor(s));

  if (chartSpeed) chartSpeed.destroy();
  chartSpeed = new Chart(document.getElementById('chart-speed'), {
    type: 'bar',
    data: {
      labels,
      datasets: [{
        label: 'km/h',
        data: speeds,
        backgroundColor: colors,
        borderRadius: 6,
        borderWidth: 0
      }]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      plugins: { legend: { display: false } },
      scales: {
        x: { grid: { display: false }, ticks: { font: { size: 11 } } },
        y: {
          grid: { color: '#1b2940' },
          ticks: { font: { size: 10 } },
          min: 0, max: 120,
          title: { display: true, text: 'km/h', color: '#6b82a0', font: { size: 10 } }
        }
      }
    }
  });
}

// ── Table render ─────────────────────────────────────────────────────────────
function render() {
  const county = document.getElementById('f-county').value;
  const cond   = document.getElementById('f-cond').value;
  let filtered = allCond;
  if (county) filtered = filtered.filter(d => (d.county_codes||[]).includes(county));
  if (cond)   filtered = filtered.filter(d => d.site_type === cond);

  document.getElementById('count-line').textContent =
    filtered.length.toLocaleString('sv-SE') + ' vägsegment' + (county||cond ? ' (filtrerade)' : '');

  const tbody = document.getElementById('cond-tbody');
  if (!filtered.length) {
    tbody.innerHTML = '<tr><td colspan="6" class="empty">Inga segment för detta urval.</td></tr>';
    return;
  }
  tbody.innerHTML = filtered.slice(0,500).map(d => {
    const counties = (d.county_codes||[]).map(c =>
      `<a class="county-chip" href="/lan/${esc(c)}">${esc(COUNTY_NAMES[c]||c.toUpperCase())}</a>`).join(' ');
    return `<tr>
      <td class="td-road">${esc(d.road_number||'—')}</td>
      <td class="td-loc" title="${esc(d.title||'')}"><span style="max-width:260px;display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(d.title||'—')}</span></td>
      <td><span class="cond-badge ${esc(condClass(d.severity))}">${esc(d.site_type||'—')}</span></td>
      <td class="td-loc">${esc(d.description||'')}</td>
      <td>${counties}</td>
      <td style="font-family:'DM Mono',monospace;font-size:.72rem;color:var(--muted)">${esc(fmtTime(d.start_time))}</td>
    </tr>`;
  }).join('');
}

// ── Load ─────────────────────────────────────────────────────────────────────
async function load() {
  const [condRes, flowRes] = await Promise.allSettled([
    fetch('/api/v1/trafik').then(r => r.ok ? r.json() : Promise.reject(r.status)),
    fetch('/api/v1/trafik/flow').then(r => r.ok ? r.json() : Promise.reject(r.status))
  ]);

  // Road conditions
  if (condRes.status === 'fulfilled') {
    allCond = condRes.value.data || [];
    document.getElementById('s-segments').textContent = allCond.length.toLocaleString('sv-SE');

    const condTypes = [...new Set(allCond.map(d=>d.site_type).filter(Boolean))].sort();
    const sel = document.getElementById('f-cond');
    condTypes.forEach(ct => { const o=document.createElement('option'); o.value=ct; o.textContent=ct; sel.appendChild(o); });

    buildCondPie(allCond);
    render();
  } else {
    document.getElementById('cond-tbody').innerHTML = '<tr><td colspan="6" class="empty">Kunde inte ladda väglagsdata.</td></tr>';
    document.getElementById('count-line').textContent = '';
  }

  // Traffic flow
  if (flowRes.status === 'fulfilled') {
    const flowData = flowRes.value.data || [];
    renderFlow(flowData);
    buildSpeedChart(flowData);
  } else {
    document.getElementById('flow-grid').innerHTML = '<div class="empty">Trafikflödesdata är inte tillgänglig.</div>';
  }
}

document.getElementById('f-county').addEventListener('change', render);
document.getElementById('f-cond').addEventListener('change', render);
load();
