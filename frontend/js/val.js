// ── Constants ──────────────────────────────────────────────────────────────
const PC = {
  S:  {hex:'#e0302a',r:224,g:48,b:42,   name:'Socialdemokraterna'},
  M:  {hex:'#66bfe6',r:102,g:191,b:230, name:'Moderaterna'},
  SD: {hex:'#4e83a3',r:78,g:131,b:163,  name:'Sverigedemokraterna'},
  C:  {hex:'#009933',r:0,g:153,b:51,    name:'Centerpartiet'},
  V:  {hex:'#b22222',r:178,g:34,b:34,   name:'Vänsterpartiet'},
  KD: {hex:'#004b8d',r:0,g:75,b:141,    name:'Kristdemokraterna'},
  L:  {hex:'#007fc3',r:0,g:127,b:195,   name:'Liberalerna'},
  MP: {hex:'#71a860',r:113,g:168,b:96,  name:'Miljöpartiet'},
};
const PARTIES = ['S','M','SD','C','V','KD','L','MP'];
const YEARS_RD = [2002,2006,2010,2014,2018,2022];
const YEARS_RF = [2006,2010,2014,2018,2022];

Chart.defaults.color = '#8892a4';
Chart.defaults.borderColor = '#1c2840';
Chart.defaults.font.family = "'DM Sans', sans-serif";

// ── State ──────────────────────────────────────────────────────────────────
let rdNat   = {};  // party → [{year,share,mandates}]
let rfNat   = {};  // party → [{year,share}]
let turnout = [];  // [{year,turnout}]
let vkData  = {};  // party → [{area,areaName,share}]
let vkAvg   = {};  // party → national 2022 share

// ── Load ───────────────────────────────────────────────────────────────────
async function loadAll() {
  const [natR, toR, vkR, rfR] = await Promise.all([
    fetch('/api/v1/val/national').then(r=>r.json()),
    fetch('/api/v1/val/turnout?area=riket').then(r=>r.json()),
    fetch('/api/v1/val/valkrets').then(r=>r.json()),
    fetch('/api/v1/val/national?area=riket_rf').then(r=>r.json()).catch(()=>({data:[]}))
  ]);

  for (const row of (natR.data||[])) {
    if (!rdNat[row.party]) rdNat[row.party]=[];
    rdNat[row.party].push({year:row.year,share:row.vote_share,mandates:row.mandates||0});
  }
  turnout = (toR.data||[]).sort((a,b)=>a.year-b.year);
  for (const row of (vkR.data||[])) {
    if (!vkData[row.party]) vkData[row.party]=[];
    vkData[row.party].push({area:row.area,areaName:row.area_name,share:row.vote_share});
  }
  for (const p of PARTIES) {
    const rec=(rdNat[p]||[]).find(r=>r.year===2022);
    if (rec) vkAvg[p]=rec.share;
  }
  for (const row of (rfR.data||[])) {
    if (!rfNat[row.party]) rfNat[row.party]=[];
    rfNat[row.party].push({year:row.year,share:row.vote_share});
  }

  buildPartyMiniCharts();
  buildTrendChart();
  buildCompChart('S');
  buildValkrets('SD');
  buildTurnoutChart();
  buildMandateChart();
  buildChips();
  buildCompChips();
  buildVkChips();
}

// ── Per-party mini charts ──────────────────────────────────────────────────
function buildPartyMiniCharts() {
  const grid = document.getElementById('partyGrid');
  grid.innerHTML = '';
  PARTIES.forEach(p => {
    const col = PC[p];
    const rows = (rdNat[p]||[]).sort((a,b)=>a.year-b.year);
    const shares = YEARS_RD.map(y=>{const r=rows.find(x=>x.year===y);return r?r.share:null;});
    const latest = shares.filter(x=>x!==null).slice(-1)[0] || 0;
    const first  = shares.filter(x=>x!==null)[0] || 0;
    const trend  = latest - first;
    const trendStr = (trend>=0?'+':'')+trend.toFixed(1)+' pp';
    const trendCol = trend>0?col.hex:(trend<0?'#ff5050':'#8892a4');

    const card = document.createElement('div');
    card.className = 'party-card';
    card.innerHTML = `
      <div class="party-card-header">
        <span class="party-abbr" style="color:${col.hex}">${p}</span>
        <span class="party-name">${col.name}</span>
        <span class="party-latest" style="color:${trendCol}">${trendStr}</span>
      </div>
      <canvas id="mini_${p}" height="90"></canvas>`;
    grid.appendChild(card);

    // Build after DOM insertion
    requestAnimationFrame(() => {
      const ctx = document.getElementById(`mini_${p}`).getContext('2d');
      const bg = shares.map(s => s===null?'transparent':
        `rgba(${col.r},${col.g},${col.b},${s===latest?0.85:0.5})`);
      new Chart(ctx, {
        type:'bar',
        data:{
          labels: YEARS_RD,
          datasets:[{
            data: shares,
            backgroundColor: bg,
            borderColor: col.hex,
            borderWidth: 1,
            borderRadius: 2,
          }]
        },
        options:{
          responsive:true,
          animation:{duration:600},
          plugins:{legend:{display:false},tooltip:{callbacks:{label:c=>` ${c.parsed.y?.toFixed(1)}%`}}},
          scales:{
            x:{grid:{display:false},ticks:{font:{size:9},color:'#8892a4'}},
            y:{grid:{color:'rgba(255,255,255,0.04)'},ticks:{font:{size:9},color:'#8892a4',callback:v=>`${v}%`},min:0}
          }
        }
      });
    });
  });
}

// ── Main trend chart ───────────────────────────────────────────────────────
function buildTrendChart() {
  const ctx = document.getElementById('trendChart').getContext('2d');
  const datasets = PARTIES.map(p => {
    const col = PC[p];
    const rows = (rdNat[p]||[]).sort((a,b)=>a.year-b.year);
    return {
      label: p,
      data: YEARS_RD.map(y=>{const r=rows.find(x=>x.year===y);return r?r.share:null;}),
      borderColor: col.hex,
      backgroundColor: `rgba(${col.r},${col.g},${col.b},0.07)`,
      borderWidth: 2.5,
      pointRadius: 4,
      pointHoverRadius: 7,
      tension: 0.35,
      fill: false,
    };
  });

  new Chart(ctx, {
    type:'line',
    data:{labels:YEARS_RD, datasets},
    options:{
      responsive:true,
      interaction:{mode:'index',intersect:false},
      plugins:{
        legend:{display:false},
        tooltip:{callbacks:{label:c=>` ${c.dataset.label}: ${c.parsed.y?.toFixed(1)}%`}}
      },
      scales:{
        x:{grid:{color:'rgba(255,255,255,0.05)'}},
        y:{grid:{color:'rgba(255,255,255,0.05)'},ticks:{callback:v=>`${v}%`},min:0}
      }
    }
  });
}

// ── Chip toggle for trend chart ────────────────────────────────────────────
function buildChips() {
  const container = document.getElementById('partyChips');
  PARTIES.forEach((p, i) => {
    const col = PC[p];
    const btn = document.createElement('button');
    btn.className = 'chip on';
    btn.textContent = p;
    btn.style.cssText = `border-color:${col.hex};background:rgba(${col.r},${col.g},${col.b},0.18);color:#fff`;
    btn.addEventListener('click', () => {
      const chart = Chart.getChart('trendChart');
      if (!chart) return;
      const visible = chart.isDatasetVisible(i);
      chart.setDatasetVisibility(i, !visible);
      chart.update('none');
      if (chart.isDatasetVisible(i)) {
        btn.classList.add('on');
        btn.style.background = `rgba(${col.r},${col.g},${col.b},0.18)`;
        btn.style.color = '#fff';
        btn.style.borderColor = col.hex;
      } else {
        btn.classList.remove('on');
        btn.style.background = 'transparent';
        btn.style.color = 'var(--muted)';
        btn.style.borderColor = 'var(--border)';
      }
    });
    container.appendChild(btn);
  });
}

// ── Riksdag vs Region comparison chart ────────────────────────────────────
let activeCompParty = 'S';

function buildCompChart(party) {
  const canvasId = 'compChart';
  const old = Chart.getChart(canvasId);
  if (old) old.destroy();

  const ctx = document.getElementById(canvasId).getContext('2d');
  const col = PC[party];
  const rdRows = (rdNat[party]||[]).sort((a,b)=>a.year-b.year);
  const rfRows = (rfNat[party]||[]).sort((a,b)=>a.year-b.year);

  // Use years where both exist
  const sharedYears = YEARS_RF;
  const rdVals = sharedYears.map(y=>{const r=rdRows.find(x=>x.year===y);return r?r.share:null;});
  const rfVals = sharedYears.map(y=>{const r=rfRows.find(x=>x.year===y);return r?r.share:null;});

  new Chart(ctx, {
    type:'line',
    data:{
      labels: sharedYears,
      datasets:[
        {
          label:'Riksdagsval',
          data: rdVals,
          borderColor: col.hex,
          backgroundColor: `rgba(${col.r},${col.g},${col.b},0.1)`,
          borderWidth:2.5, pointRadius:5, tension:0.3, fill:false,
        },
        {
          label:'Regionval',
          data: rfVals,
          borderColor: col.hex,
          backgroundColor: `rgba(${col.r},${col.g},${col.b},0.05)`,
          borderWidth:2, pointRadius:4, borderDash:[5,3], tension:0.3, fill:false,
        }
      ]
    },
    options:{
      responsive:true,
      interaction:{mode:'index',intersect:false},
      plugins:{
        legend:{display:true,labels:{color:'#8892a4',boxWidth:24,font:{size:12}}},
        tooltip:{callbacks:{label:c=>` ${c.dataset.label}: ${c.parsed.y?.toFixed(1)}%`}}
      },
      scales:{
        x:{grid:{color:'rgba(255,255,255,0.05)'}},
        y:{grid:{color:'rgba(255,255,255,0.05)'},ticks:{callback:v=>`${v}%`},min:0}
      }
    }
  });
}

function buildCompChips() {
  const container = document.getElementById('compChips');
  PARTIES.forEach(p => {
    const col = PC[p];
    const btn = document.createElement('button');
    btn.className = 'chip' + (p===activeCompParty?' on':'');
    btn.textContent = p;
    if (p===activeCompParty) {
      btn.style.cssText = `border-color:${col.hex};background:rgba(${col.r},${col.g},${col.b},0.18);color:#fff`;
    } else {
      btn.style.borderColor = col.hex;
    }
    btn.addEventListener('click', () => {
      activeCompParty = p;
      container.querySelectorAll('.chip').forEach((b, i) => {
        const pp = PARTIES[i];
        const cc = PC[pp];
        b.classList.toggle('on', pp===p);
        if (pp===p) {
          b.style.background = `rgba(${cc.r},${cc.g},${cc.b},0.18)`;
          b.style.color = '#fff';
          b.style.borderColor = cc.hex;
        } else {
          b.style.background = 'transparent';
          b.style.color = 'var(--muted)';
          b.style.borderColor = cc.hex;
        }
      });
      buildCompChart(p);
    });
    container.appendChild(btn);
  });
}

// ── Valkrets bars ──────────────────────────────────────────────────────────
let activeVkParty = 'SD';

function buildVkChips() {
  const container = document.getElementById('vkChips');
  PARTIES.forEach(p => {
    const col = PC[p];
    const btn = document.createElement('button');
    btn.className = 'chip' + (p===activeVkParty?' on':'');
    btn.textContent = p;
    if (p===activeVkParty) {
      btn.style.cssText = `border-color:${col.hex};background:rgba(${col.r},${col.g},${col.b},0.18);color:#fff`;
    } else {
      btn.style.borderColor = col.hex;
    }
    btn.addEventListener('click', () => {
      activeVkParty = p;
      container.querySelectorAll('.chip').forEach((b, i) => {
        const pp = PARTIES[i];
        const cc = PC[pp];
        b.classList.toggle('on', pp===p);
        if (pp===p) {
          b.style.background = `rgba(${cc.r},${cc.g},${cc.b},0.18)`;
          b.style.color = '#fff';
          b.style.borderColor = cc.hex;
        } else {
          b.style.background = 'transparent';
          b.style.color = 'var(--muted)';
          b.style.borderColor = cc.hex;
        }
      });
      buildValkrets(p);
    });
    container.appendChild(btn);
  });
}

function buildValkrets(party) {
  const container = document.getElementById('vkBars');
  container.innerHTML = '';
  const rows = (vkData[party]||[]).sort((a,b)=>b.share-a.share);
  if (!rows.length) {
    container.innerHTML = '<p style="color:var(--muted);font-size:13px;padding:8px 0">Laddar…</p>';
    return;
  }
  const avg = vkAvg[party]||0;
  const max = Math.max(...rows.map(r=>r.share));
  const range = max||1;
  const col = PC[party]||{hex:'#666',r:102,g:102,b:128};
  const avgPct = (avg/range*100).toFixed(2);

  const grid = document.createElement('div');
  grid.className = 'hbar-grid';

  rows.forEach((row, i) => {
    const fillPct = (row.share/range*100).toFixed(2);

    const rank = document.createElement('div');
    rank.className = 'hbar-rank';
    rank.textContent = i+1;

    const name = document.createElement('div');
    name.className = 'hbar-name';
    name.textContent = row.areaName;
    name.title = row.areaName;

    const track = document.createElement('div');
    track.className = 'hbar-track';
    const fill = document.createElement('div');
    fill.className = 'hbar-fill';
    fill.style.width = '0%';
    fill.style.background = `rgba(${col.r},${col.g},${col.b},0.8)`;
    const avgLine = document.createElement('div');
    avgLine.className = 'hbar-avg';
    avgLine.style.left = avgPct+'%';
    avgLine.title = `Rikssnitt ${avg.toFixed(1)}%`;
    track.appendChild(fill);
    track.appendChild(avgLine);

    const val = document.createElement('div');
    val.className = 'hbar-val';
    val.textContent = row.share.toFixed(1)+'%';

    grid.appendChild(rank);
    grid.appendChild(name);
    grid.appendChild(track);
    grid.appendChild(val);

    requestAnimationFrame(()=>setTimeout(()=>{fill.style.width=fillPct+'%';}, i*16));
  });
  container.appendChild(grid);
}

// ── Turnout chart ──────────────────────────────────────────────────────────
function buildTurnoutChart() {
  const ctx = document.getElementById('turnoutChart').getContext('2d');
  const rows = turnout.sort((a,b)=>a.year-b.year);
  new Chart(ctx,{
    type:'line',
    data:{
      labels:rows.map(r=>r.year),
      datasets:[{
        label:'Valdeltagande %',
        data:rows.map(r=>r.turnout),
        borderColor:'rgb(45,212,160)',
        backgroundColor:'rgba(45,212,160,0.08)',
        borderWidth:2.5,pointRadius:5,tension:0.3,fill:true
      }]
    },
    options:{
      responsive:true,
      plugins:{legend:{display:false},tooltip:{callbacks:{label:c=>` ${c.parsed.y.toFixed(2)}%`}}},
      scales:{
        x:{grid:{color:'rgba(255,255,255,0.05)'}},
        y:{grid:{color:'rgba(255,255,255,0.05)'},ticks:{callback:v=>`${v}%`},min:75,max:92}
      }
    }
  });
}

// ── Mandate chart ──────────────────────────────────────────────────────────
function buildMandateChart() {
  const ctx = document.getElementById('mandateChart').getContext('2d');
  const datasets = PARTIES.map(p=>{
    const col=PC[p];
    const rows=(rdNat[p]||[]).sort((a,b)=>a.year-b.year);
    return{
      label:p,
      data:YEARS_RD.map(y=>{const r=rows.find(x=>x.year===y);return r?r.mandates:0;}),
      backgroundColor:`rgba(${col.r},${col.g},${col.b},0.75)`,
      borderColor:col.hex,borderWidth:1,borderRadius:2
    };
  });

  new Chart(ctx,{
    type:'bar',
    data:{labels:YEARS_RD,datasets},
    options:{
      responsive:true,
      plugins:{legend:{display:false},tooltip:{callbacks:{label:c=>` ${c.dataset.label}: ${c.parsed.y} mandat`}}},
      scales:{
        x:{stacked:true,grid:{color:'rgba(255,255,255,0.05)'}},
        y:{stacked:true,min:0,max:360,grid:{color:'rgba(255,255,255,0.05)'},ticks:{callback:v=>v}}
      }
    },
    plugins:[{
      afterDraw(chart){
        const yPx=chart.scales.y.getPixelForValue(175);
        const ctx2=chart.ctx;
        ctx2.save();
        ctx2.strokeStyle='rgba(255,200,50,0.5)';
        ctx2.lineWidth=1.5;
        ctx2.setLineDash([5,4]);
        ctx2.beginPath();
        ctx2.moveTo(chart.chartArea.left,yPx);
        ctx2.lineTo(chart.chartArea.right,yPx);
        ctx2.stroke();
        ctx2.fillStyle='rgba(255,200,50,0.7)';
        ctx2.font='11px DM Sans';
        ctx2.fillText('175 (majoritetsgräns)',chart.chartArea.left+8,yPx-5);
        ctx2.restore();
      }
    }]
  });
}

loadAll();
