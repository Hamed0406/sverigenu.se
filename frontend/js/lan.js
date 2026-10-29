'use strict';

const COUNTY_NAMES = {
  ab:'Stockholm', c:'Uppsala', d:'Södermanland', e:'Östergötland',
  f:'Jönköping', g:'Kronoberg', h:'Kalmar', i:'Gotland', k:'Blekinge',
  m:'Skåne', n:'Halland', o:'Västra Götaland', s:'Värmland',
  t:'Örebro', u:'Västmanland', w:'Dalarna', x:'Gävleborg',
  y:'Västernorrland', z:'Jämtland', ac:'Västerbotten', bd:'Norrbotten'
};
const COUNTY_COLORS = {
  ab:'#4fa3e8', o:'#2dd4a0', m:'#7c6cff', c:'#79c0ff',
  e:'#d29922', f:'#56d364', n:'#ff7b72', bd:'#38b6ff',
  x:'#bc8cff', y:'#f78166', z:'#3fb950', w:'#e3b341',
  d:'#6cb6ff', g:'#57ab5a', h:'#e0823d', i:'#b083f0',
  k:'#ec8e7b', s:'#54aeff', t:'#6fdd8b', u:'#daaa3f', ac:'#8ddb8c'
};

const code = (location.pathname.match(/\/lan\/([a-z]{1,2})$/) || [])[1] || '';
const countyName = COUNTY_NAMES[code];
const accentColor = COUNTY_COLORS[code] || '#2dd4a0';

// Update OG meta tags dynamically for this county
if (countyName) {
  const ogUrl   = document.querySelector('meta[property="og:url"]');
  const ogTitle = document.querySelector('meta[property="og:title"]');
  const twTitle = document.querySelector('meta[name="twitter:title"]');
  if (ogUrl)   ogUrl.content   = 'https://sverigedata.nu' + location.pathname;
  if (ogTitle) ogTitle.content = countyName + ' — statistik och fakta';
  if (twTitle) twTitle.content = countyName + ' — statistik och fakta';
}

if (!countyName) {
  document.getElementById('main').innerHTML =
    '<div class="error-page"><h1>Länet hittades inte</h1>' +
    '<p>Koden &ldquo;' + esc(code) + '&rdquo; matchar inget av Sveriges 21 l&auml;n.<br>' +
    'G&aring; tillbaka till <a href="/">startsidan</a> och klicka p&aring; ett l&auml;n p&aring; kartan.</p></div>';
  document.title = '404 — Sverige Nu';
} else {
  document.title = countyName + ' — Sverige Nu';
  document.documentElement.style.setProperty('--ac', accentColor);
  document.getElementById('footer').removeAttribute('hidden');
  init();
}

function esc(s) {
  return String(s)
    .replace(/&/g,'&amp;').replace(/</g,'&lt;')
    .replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function safeUrl(url) {
  return (typeof url === 'string' && url.startsWith('https://polisen.se/')) ? url : null;
}

const fmt = n => (n == null ? '—' : Math.round(n).toLocaleString('sv-SE'));
const fmtDec = (n, d) => (n == null ? '—' : n.toFixed(d == null ? 1 : d).replace('.', ','));

function dateStr(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return d.toLocaleDateString('sv-SE', { month:'short', day:'numeric' })
    + ' ' + d.toLocaleTimeString('sv-SE', { hour:'2-digit', minute:'2-digit' });
}

function buildCountySelect() {
  const sorted = Object.entries(COUNTY_NAMES).sort((a,b) => a[1].localeCompare(b[1], 'sv'));
  return '<select class="county-select" id="countySelect">'
    + '<option value="">— Byt län —</option>'
    + sorted.map(([k, v]) =>
        '<option value="' + k + '"' + (k === code ? ' selected' : '') + '>' + v + '</option>'
      ).join('')
    + '</select>';
}

function init() {
  document.getElementById('main').innerHTML = `
    <div class="hero">
      <p class="eyebrow rv rv-1">
        <a href="/">Sverige Nu</a> &nbsp;/&nbsp; Länen &nbsp;/&nbsp; ${esc(countyName)}
      </p>
      <h1 class="rv rv-2">${esc(countyName)} &mdash; <em>ett län i siffror</em></h1>
      <p class="lead rv rv-3">Befolkning, inkomst, brottslighet, hälsa och polishändelser för ${esc(countyName)}s län.</p>
      ${buildCountySelect()}
      <div class="stat-band rv rv-4">
        <div class="band-cell primary">
          <div class="band-num" id="heroPop">—</div>
          <div class="band-label">Folkbokförda</div>
          <div class="band-sub" id="heroPopSub"></div>
        </div>
        <div class="band-cell">
          <div class="band-num" id="heroTemp" style="color:var(--blue)">—</div>
          <div class="band-label">Medeltemperatur</div>
          <div class="band-sub" id="heroTempSub">nu</div>
        </div>
        <div class="band-cell">
          <div class="band-num" id="heroPolis" style="color:var(--red)">—</div>
          <div class="band-label">Polishändelser</div>
          <div class="band-sub">senaste 7 dagarna</div>
        </div>
        <div class="band-cell">
          <div class="band-num" id="heroBrott" style="color:var(--ac)">—</div>
          <div class="band-label">Brott / 100 000 inv</div>
          <div class="band-sub" id="heroBrottSub">BRÅ</div>
        </div>
      </div>
    </div>

    <section class="section" id="profilSection">
      <div class="section-header">
        <h2 class="section-title">Länsanalys <em>i jämförelse</em></h2>
        <span class="section-sub">Ranking av 21 län</span>
      </div>
      <div class="rank-cards" id="rankCards"></div>
      <div class="two-col" style="margin-top:32px">
        <div class="chart-card">
          <div style="font-family:'DM Mono',monospace;font-size:.64rem;letter-spacing:.12em;text-transform:uppercase;color:var(--dim);margin-bottom:4px">Socioekonomisk profil</div>
          <p style="font-size:.78rem;color:var(--muted);margin:0 0 14px;line-height:1.55">
            Varje axel visar hur <strong style="color:var(--text)">${esc(countyName)}</strong> presterar jämfört med alla 21 län.
            <strong style="color:var(--text)">Yttre kant = bäst</strong>, mitten = sämst.
            Poängen är omräknat från ranking: plats 1 av 21 ger 100 p, plats 21 ger 5 p.
          </p>
          <div class="chart-wrap" style="max-height:300px"><canvas id="radarChart"></canvas></div>
          <div class="radar-legend">
            <div class="rl-row"><span class="rl-axis">Inkomst</span><span class="rl-src">Medianinkomst 20+ år · SCB HE0110</span></div>
            <div class="rl-row"><span class="rl-axis">Sysselsättning</span><span class="rl-src">Andel arbetslösa 18–64 år · Kolada N03920 (lägre = bättre)</span></div>
            <div class="rl-row"><span class="rl-axis">Utbildning</span><span class="rl-src">Meritvärde åk 9 · Kolada N15504</span></div>
            <div class="rl-row"><span class="rl-axis">Hälsa</span><span class="rl-src">Medellivslängd · Folkhälsomyndigheten</span></div>
            <div class="rl-row"><span class="rl-axis">Trygghet</span><span class="rl-src">Anmälda brott / 100 000 inv · BRÅ (lägre = bättre)</span></div>
            <div class="rl-row"><span class="rl-axis">Socialbidrag</span><span class="rl-src">Utbetalt ekonomiskt bistånd kr/inv · Kolada N31004 (lägre = bättre)</span></div>
          </div>
        </div>
        <div class="chart-card">
          <div style="font-family:'DM Mono',monospace;font-size:.64rem;letter-spacing:.12em;text-transform:uppercase;color:var(--dim);margin-bottom:12px">Riksdagsval 2022</div>
          <div id="politicsBar2022"></div>
          <p class="chart-note" style="margin-top:12px">Röstandel per parti · <span id="domPartyBadge"></span></p>
        </div>
      </div>
    </section>

    <section class="section" id="politikSection">
      <div class="section-header">
        <h2 class="section-title">Politik <em>över tid</em></h2>
        <span class="section-sub">Riksdagsval 2006–2022 · SCB ME0104T3</span>
      </div>
      <div class="chart-card">
        <div class="chart-wrap sm"><canvas id="politikChart"></canvas></div>
        <p class="chart-note">Röstandel (%) per riksdagsparti · per valkrets · SCB + Valmyndigheten</p>
      </div>
    </section>

    <section class="section" id="vaderSection" style="display:none">
      <div class="section-header">
        <h2 class="section-title">Väder <em>just nu</em></h2>
        <span class="section-sub" id="vaderSub">SMHI · stationsdata</span>
      </div>
      <div class="kpi-row" id="vaderKpis"></div>
      <div class="station-grid" id="stationGrid"></div>
    </section>

    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Befolkning <em>över tid</em></h2>
        <span class="section-sub" id="befolkSub">SCB BE0101</span>
      </div>
      <div class="chart-card">
        <div class="chart-wrap"><canvas id="befolkChart"></canvas></div>
        <p class="chart-note">Folkbokförd befolkning 31 dec respektive år · Källa: SCB BE0101</p>
      </div>
    </section>

    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Inkomst <em>per invånare</em></h2>
        <span class="section-sub">SCB HE0110 · 1991–</span>
      </div>
      <div class="chart-card">
        <div class="chart-wrap sm"><canvas id="inkomstChart"></canvas></div>
        <p class="chart-note">Sammanräknad förvärvsinkomst · medelinkomst (heldragen) och medianinkomst (streckad) · 20+ år · tusentals kronor · SCB</p>
      </div>
    </section>

    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Brottslighet <em>vs riket</em></h2>
        <span class="section-sub" id="brottSub">BRÅ · 1996–</span>
      </div>
      <div class="two-col">
        <div class="chart-card">
          <div class="controls">
            <span class="ctrl-label">Mått</span>
            <div class="sex-toggle" id="brottToggle">
              <button class="sex-btn active" data-m="per100k">Per 100k</button>
              <button class="sex-btn" data-m="count">Antal</button>
            </div>
          </div>
          <div class="chart-wrap sm"><canvas id="brottChart"></canvas></div>
          <p class="chart-note">Totalt anmälda brott · ${esc(countyName)} vs rikssnitt per 100 000 invånare · BRÅ + SCB</p>
        </div>
        <div class="chart-card">
          <div style="font-family:'DM Mono',monospace;font-size: .64rem;letter-spacing:.12em;text-transform:uppercase;color:var(--dim);margin-bottom:16px" id="rankTitle">Ranking · alla 21 län</div>
          <div class="hbar-list" id="rankList"></div>
          <p class="chart-note">Anmälda brott per 100 000 invånare · senaste år · BRÅ + SCB</p>
        </div>
      </div>
    </section>

    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Hälsa <em>och livslängd</em></h2>
        <span class="section-sub">Folkhälsomyndigheten</span>
      </div>
      <div class="kpi-row" id="halsaRow">
        <div class="kpi-card">
          <div class="kpi-label">Medellivslängd · Kvinnor</div>
          <div class="kpi-val" id="halsaLifeW">—</div>
          <div class="kpi-sub" id="halsaLifeWS">år</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Medellivslängd · Män</div>
          <div class="kpi-val" id="halsaLifeM">—</div>
          <div class="kpi-sub" id="halsaLifeMS">år</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Dagligrökare</div>
          <div class="kpi-val" id="halsaSmoke">—</div>
          <div class="kpi-sub" id="halsaSmokeS">% av befolkningen</div>
        </div>
      </div>
    </section>

    <section class="section" id="arbSection">
      <div class="section-header">
        <h2 class="section-title">Arbetsmarknad <em>och jobb</em></h2>
        <span class="section-sub" id="arbSub">Arbetsförmedlingen · Kolada N03920</span>
      </div>
      <div class="kpi-row">
        <div class="kpi-card">
          <div class="kpi-label">Lediga annonser</div>
          <div class="kpi-val" id="arbAds">—</div>
          <div class="kpi-sub" id="arbAdsSub">Arbetsförmedlingen · nu</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Ranking lediga jobb</div>
          <div class="kpi-val" id="arbRank">—</div>
          <div class="kpi-sub">av 21 län</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Arbetslöshet 18–64 år</div>
          <div class="kpi-val" id="arbUnemp">—</div>
          <div class="kpi-sub" id="arbUnempSub">kommunsnitt · Kolada</div>
        </div>
      </div>
    </section>

    <section class="section" id="trafikSection" style="display:none">
      <div class="section-header">
        <h2 class="section-title">Trafik <em>i länet</em></h2>
        <span class="section-sub">Trafikverket · realtid</span>
      </div>
      <div class="trafik-summary" id="trafikSummary"></div>
    </section>

    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Senaste <em>polishändelser</em></h2>
        <span class="section-sub" id="polisSub">senaste 7 dagarna</span>
      </div>
      <div class="event-list" id="eventList">
        <div style="color:var(--dim);font-family:'DM Mono',monospace;font-size: .82rem">Laddar händelser…</div>
      </div>
    </section>

    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Kommuner <em>i länet</em></h2>
        <span class="section-sub">Kolada N03920 · arbetslöshet 18–64 år</span>
      </div>
      <div class="chart-card" style="overflow-x:auto">
        <table class="muni-table">
          <thead><tr>
            <th>#</th><th>Kommun</th><th>Arbetslöshet</th>
          </tr></thead>
          <tbody id="muniBody"></tbody>
        </table>
        <p class="chart-note">Källa: Kolada N03920 · andel arbetslösa 18–64 år · senaste period</p>
      </div>
    </section>

    <section class="section" style="margin-top:2.5rem">
      <div class="prose-grid">
        <div class="prose">
          <h2 class="section-title" style="margin-bottom:16px">Om <em>statistiken</em></h2>
          <p>Länssidan samlar data från <strong>sju olika öppna datakällor</strong>: SCB (befolkning, inkomst, BRP), BRÅ (brott), Polisen (händelser), Folkhälsomyndigheten (hälsoindikatorer), Kolada (kommunstatistik), SMHI (väder) och Trafikverket (väglag). Alla källor uppdateras automatiskt med olika intervall — från var 15:e minut (SMHI, polis, trafik) till årsvis (SCB).</p>
          <p>Datan normaliseras per invånare där det är relevant för att göra länen jämförbara. Kommunjämförelserna visar alla kommuner inom länet sorterade efter senaste tillgängliga värde. Observera att definitioner och täckningsgrad varierar något mellan källorna.</p>
        </div>
        <div>
          <h2 class="section-title" style="margin-bottom:16px">Vanliga <em>frågor</em></h2>
          <details>
            <summary>Varför skiljer sig data så mycket mellan länen?</summary>
            <p>Länen är mycket heterogena — befolkningstäthet, näringsliv, åldersstruktur och urbaniseringsgrad varierar extremt. Stockholm har 2,5 miljoner invånare; Gotland knappt 60 000. Skillnaderna i inkomst, hälsa och brottslighet speglar dessa strukturella olikheter lika mycket som politik eller styrning.</p>
          </details>
          <details>
            <summary>Hur aktuella är siffrorna?</summary>
            <p>Det beror på källan. Väder och polishändelser uppdateras i realtid (var 15:e min). Elpris var 15:e minut. Befolkning och inkomst är årsdata med 1–2 års fördröjning från SCB. Senaste synkroniseringstid finns i sidans statuspanel under /om.</p>
          </details>
          <details>
            <summary>Vad är ett 'socioekonomiskt profil-index'?</summary>
            <p>Profil-indexet på länssidan kombinerar inkomst, utbildningsnivå, förvärvsfrekvens och beroende av transfereringar till ett samlat mått (0–100). Det syftar inte till att värdera ett län som 'bättre' — det beskriver den socioekonomiska sammansättningen.</p>
          </details>
          <details>
            <summary>Ingår alla kommuner för varje mått?</summary>
            <p>Täckningen varierar. Kolada-måtten (arbetslöshet, socioekonomisk profil) täcker alla 290 kommuner. BRÅ:s brottsstatistik publiceras per polisdistrikt och kan inte alltid delas upp exakt per kommun. SCB täcker alla kommuner men med ett par års fördröjning.</p>
          </details>
        </div>
      </div>
    </section>
  `;

  document.getElementById('countySelect').addEventListener('change', function() {
    if (this.value) location.href = '/lan/' + this.value;
  });

  let brottMeasure = 'per100k';
  let brottLanAll = [];
  let brottChartObj = null;

  document.getElementById('brottToggle').querySelectorAll('.sex-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#brottToggle .sex-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      brottMeasure = btn.dataset.m;
      if (brottLanAll.length) buildBrottChart(brottLanAll);
    });
  });

  // Fetch profile + rankings in parallel with other data
  fetch('/api/v1/lan/profile?county=' + code).then(r => r.json()).then(buildProfil).catch(() => {});
  fetch('/api/v1/lan/rankings').then(r => r.json()).then(data => buildRankCards(data.data || [])).catch(() => {});

  Promise.all([
    fetch('/api/v1/lan/summary').then(r => r.json()),
    fetch('/api/v1/befolkning?county=' + code).then(r => r.json()),
    fetch('/api/v1/inkomst?county=' + code).then(r => r.json()),
    fetch('/api/v1/brott/lan').then(r => r.json()),
    fetch('/api/v1/policeevents?county=' + code).then(r => r.json()),
    fetch('/api/v1/folkhalsa?county=' + code).then(r => r.json()),
    fetch('/api/v1/kolada?county=' + code + '&kpi=N03920').then(r => r.json()),
    fetch('/api/v1/arbetsmarknad/jobs').then(r => r.json()),
  ]).then(([lanRes, befolkRes, inkomstRes, brottRes, polisRes, halsaRes, kommunRes, jobsRes]) => {
    const lanRow = (lanRes.data || []).find(r => r.code === code);
    brottLanAll = brottRes.data || [];

    buildHero(lanRow);
    buildBefolkChart(befolkRes.data || []);
    buildInkomstChart(inkomstRes.data || []);
    buildBrottChart(brottLanAll);
    buildRankBars(brottLanAll);
    buildHalsa(halsaRes.data || []);
    buildEvents(polisRes.data || []);
    buildKommun(kommunRes.data || []);
    buildArbetsmarknad(jobsRes.data || [], kommunRes.data || []);
  }).catch(err => {
    console.error('Data load failed', err);
  });

  function buildHero(lanRow) {
    if (!lanRow) return;
    if (lanRow.population) {
      document.getElementById('heroPop').textContent = fmt(lanRow.population);
      document.getElementById('heroPopSub').textContent =
        lanRow.population_year ? String(lanRow.population_year) : '';
    }
    if (lanRow.avg_temp_c != null) {
      document.getElementById('heroTemp').textContent = fmtDec(lanRow.avg_temp_c) + ' °C';
      document.getElementById('heroTempSub').textContent =
        (lanRow.station_count || '') + ' stationer · nu';
    }
    document.getElementById('heroPolis').textContent = fmt(lanRow.police_events_7d);
  }

  function buildBefolkChart(data) {
    if (!data.length) return;
    data.sort((a, b) => a.year - b.year);
    const years = data.map(d => d.year);
    document.getElementById('befolkSub').textContent =
      'SCB BE0101 · ' + years[0] + '–' + years[years.length - 1];
    new Chart(document.getElementById('befolkChart'), {
      type: 'line',
      data: {
        labels: years,
        datasets: [{
          label: countyName,
          data: data.map(d => d.population),
          borderColor: accentColor, backgroundColor: accentColor + '18',
          tension: .3, pointRadius: 0, pointHoverRadius: 5,
          fill: true, borderWidth: 2,
        }]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
            titleColor: '#e3ebf5', bodyColor: '#6b82a0',
            callbacks: { label: c => ' ' + Math.round(c.parsed.y).toLocaleString('sv-SE') + ' invånare' }
          }
        },
        scales: {
          x: { ticks: { color: '#3d5270', maxTicksLimit: 10 }, grid: { color: 'rgba(27,41,64,.4)' } },
          y: { beginAtZero: false, ticks: { color: '#3d5270', callback: v => v >= 1e6 ? (v/1e6).toFixed(1) + 'M' : Math.round(v/1e3) + 'k' }, grid: { color: 'rgba(27,41,64,.4)' } }
        }
      }
    });
  }

  function buildInkomstChart(data) {
    if (!data.length) return;
    const meanByY = {}, medByY = {};
    data.forEach(d => {
      if (d.metric === 'mean')   meanByY[d.year] = d.value;
      if (d.metric === 'median') medByY[d.year]  = d.value;
    });
    const years = [...new Set(data.map(d => d.year))].sort((a,b) => a - b);
    new Chart(document.getElementById('inkomstChart'), {
      type: 'line',
      data: {
        labels: years,
        datasets: [
          {
            label: 'Medelinkomst',
            data: years.map(y => meanByY[y] ?? null),
            borderColor: accentColor, backgroundColor: 'transparent',
            tension: .3, pointRadius: 0, pointHoverRadius: 4, borderWidth: 2.5, spanGaps: true,
          },
          {
            label: 'Medianinkomst',
            data: years.map(y => medByY[y] ?? null),
            borderColor: accentColor + '60', backgroundColor: 'transparent',
            tension: .3, pointRadius: 0, pointHoverRadius: 4,
            borderWidth: 1.5, borderDash: [5, 3], spanGaps: true,
          }
        ]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { labels: { color: '#6b82a0', font: { family: 'DM Mono', size: 11 }, boxWidth: 18 } },
          tooltip: {
            backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
            callbacks: { label: c => ' ' + c.dataset.label + ': ' + (c.parsed.y == null ? '—' : c.parsed.y.toFixed(1)) + ' tkr' }
          }
        },
        scales: {
          x: { ticks: { color: '#3d5270', maxTicksLimit: 8 }, grid: { color: 'rgba(27,41,64,.4)' } },
          y: { beginAtZero: false, ticks: { color: '#3d5270', callback: v => v + ' tkr' }, grid: { color: 'rgba(27,41,64,.4)' } }
        }
      }
    });
  }

  function buildBrottChart(brottLan) {
    const countyRows = brottLan.filter(d => d.county === code && d.category === 'totalt').sort((a,b) => a.year - b.year);
    if (!countyRows.length) return;

    const riketByY = new Map();
    brottLan.forEach(d => {
      if (d.category !== 'totalt' || d.count == null) return;
      const e = riketByY.get(d.year) || { count: 0, pop: 0 };
      e.count += d.count;
      if (d.population) e.pop += d.population;
      riketByY.set(d.year, e);
    });

    const latestYear = Math.max(...countyRows.map(d => d.year));
    document.getElementById('brottSub').textContent = 'BRÅ · ' + countyRows[0].year + '–' + latestYear;
    document.getElementById('heroBrott').textContent = fmt((countyRows.find(d => d.year === latestYear) || {}).per_100k);
    document.getElementById('heroBrottSub').textContent = 'BRÅ · ' + latestYear;

    const years = countyRows.map(d => d.year);
    const cVals = years.map(y => {
      const r = countyRows.find(d => d.year === y);
      if (!r) return null;
      return brottMeasure === 'count' ? r.count : r.per_100k;
    });
    const rVals = years.map(y => {
      const e = riketByY.get(y);
      if (!e) return null;
      if (brottMeasure === 'count') return e.count;
      return e.pop > 0 ? e.count / e.pop * 100000 : null;
    });

    const ctx = document.getElementById('brottChart');
    if (brottChartObj) brottChartObj.destroy();
    brottChartObj = new Chart(ctx, {
      type: 'line',
      data: {
        labels: years,
        datasets: [
          {
            label: countyName,
            data: cVals,
            borderColor: accentColor, backgroundColor: accentColor + '18',
            tension: .3, pointRadius: 0, pointHoverRadius: 5, borderWidth: 2.5, fill: false, spanGaps: true,
          },
          {
            label: 'Rikssnitt',
            data: rVals,
            borderColor: '#3d5270', backgroundColor: 'transparent',
            tension: .3, pointRadius: 0, pointHoverRadius: 4,
            borderWidth: 1.5, borderDash: [5, 3], spanGaps: true,
          }
        ]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { labels: { color: '#6b82a0', font: { family: 'DM Mono', size: 11 }, boxWidth: 18 } },
          tooltip: {
            backgroundColor: '#0f1726', borderColor: '#1b2940', borderWidth: 1,
            callbacks: { label: c => ' ' + c.dataset.label + ': ' + (c.parsed.y == null ? '—' : Math.round(c.parsed.y).toLocaleString('sv-SE')) }
          }
        },
        scales: {
          x: { ticks: { color: '#3d5270', maxTicksLimit: 10 }, grid: { color: 'rgba(27,41,64,.4)' } },
          y: { beginAtZero: false, ticks: { color: '#3d5270', callback: v => Math.round(v).toLocaleString('sv-SE') }, grid: { color: 'rgba(27,41,64,.4)' } }
        }
      }
    });
  }

  function buildRankBars(brottLan) {
    const totRows = brottLan.filter(d => d.category === 'totalt' && d.per_100k != null);
    if (!totRows.length) return;
    const latestYear = Math.max(...totRows.map(d => d.year));
    document.getElementById('rankTitle').textContent = 'Ranking · alla 21 län · ' + latestYear;
    const rows = totRows
      .filter(d => d.year === latestYear)
      .map(d => ({ county: d.county, name: COUNTY_NAMES[d.county] || d.county, value: d.per_100k }))
      .sort((a, b) => b.value - a.value);
    const el = document.getElementById('rankList');
    el.innerHTML = '';
    const max = Math.max(...rows.map(r => r.value));
    rows.forEach((r, i) => {
      const isThis = r.county === code;
      const pct = max > 0 ? r.value / max * 100 : 0;
      const row = document.createElement('div');
      row.className = 'hbar-row';
      row.innerHTML =
        '<div class="hbar-rank">' + (i + 1) + '</div>' +
        '<div class="hbar-name' + (isThis ? ' hl' : '') + '">' + esc(r.name) + '</div>' +
        '<div class="hbar-track"><div class="hbar-fill' + (isThis ? ' hl' : '') + '"></div></div>' +
        '<div class="hbar-val' + (isThis ? ' hl' : '') + '">' + fmt(r.value) + '</div>';
      el.appendChild(row);
      setTimeout(() => {
        row.querySelector('.hbar-fill').style.width = pct + '%';
        row.classList.add('in');
      }, 60 + i * 28);
    });
  }

  function buildHalsa(data) {
    function latest(metric, sex) {
      return data.filter(d => d.metric === metric && d.sex === sex)
        .sort((a, b) => b.period.localeCompare(a.period))[0] || null;
    }
    const lifeW = latest('lifeexp', 'women');
    const lifeM = latest('lifeexp', 'men');
    const smoke = latest('smoking', 'total');
    if (lifeW) {
      document.getElementById('halsaLifeW').textContent = fmtDec(lifeW.value) + ' år';
      document.getElementById('halsaLifeWS').textContent = lifeW.period;
    }
    if (lifeM) {
      document.getElementById('halsaLifeM').textContent = fmtDec(lifeM.value) + ' år';
      document.getElementById('halsaLifeMS').textContent = lifeM.period;
    }
    if (smoke) {
      document.getElementById('halsaSmoke').textContent = fmtDec(smoke.value) + ' %';
      document.getElementById('halsaSmokeS').textContent = smoke.period + ' · dagligrökare';
    }
  }

  function buildEvents(data) {
    const el = document.getElementById('eventList');
    const events = data.slice(0, 20);
    if (!events.length) {
      el.innerHTML = '<div style="color:var(--dim);font-family:\'DM Mono\',monospace;font-size: .82rem">Inga händelser hittades för detta län.</div>';
      return;
    }
    document.getElementById('polisSub').textContent = events.length + ' händelser visas';
    el.innerHTML = events.map(e => {
      const url = safeUrl(e.url);
      const nameEl = url
        ? '<a href="' + esc(url) + '" target="_blank" rel="noopener">' + esc(e.name || e.type || '') + '</a>'
        : esc(e.name || e.type || '');
      return '<div class="event-item">' +
        '<div class="event-time">' + esc(dateStr(e.event_time)) + '</div>' +
        '<div class="event-body">' +
        '<div class="event-type">' + esc(e.type || 'Händelse') + '</div>' +
        '<div class="event-name">' + nameEl + '</div>' +
        (e.location ? '<div class="event-loc">' + esc(e.location) + '</div>' : '') +
        '</div></div>';
    }).join('');
  }

  function buildArbetsmarknad(allJobs, unempData) {
    const thisCounty = allJobs.find(j => j.county_code === code);
    if (thisCounty) {
      document.getElementById('arbAds').textContent = fmt(thisCounty.total_ads);
      const ranked = [...allJobs].sort((a, b) => b.total_ads - a.total_ads);
      const rank = ranked.findIndex(j => j.county_code === code) + 1;
      document.getElementById('arbRank').textContent = '#' + rank;
    }
    if (unempData.length) {
      const vals = unempData.filter(d => d.value != null).map(d => d.value);
      if (vals.length) {
        const avg = vals.reduce((s, v) => s + v, 0) / vals.length;
        document.getElementById('arbUnemp').textContent = fmtDec(avg) + ' %';
        document.getElementById('arbUnempSub').textContent = 'snitt ' + vals.length + ' kommuner · senaste period';
      }
    }
  }

  function buildKommun(data) {
    const tbody = document.getElementById('muniBody');
    if (!data.length) {
      tbody.innerHTML = '<tr><td colspan="3" style="color:var(--dim);font-size: .82rem;padding:16px">Ingen kommundata hittades.</td></tr>';
      return;
    }
    const sorted = [...data].filter(d => d.value != null).sort((a, b) => b.value - a.value);
    tbody.innerHTML = sorted.map((r, i) =>
      '<tr>' +
      '<td class="rank">' + (i + 1) + '</td>' +
      '<td class="mname">' + esc(r.name) + '</td>' +
      '<td class="mval">' + fmtDec(r.value) + ' %</td>' +
      '</tr>'
    ).join('');
  }

  // ── Väder + Trafik (non-blocking) ───────────────────────────────────────────
  Promise.allSettled([
    fetch('/api/v1/weather?county=' + code).then(r => r.json()),
    fetch('/api/v1/trafik?county=' + code).then(r => r.json()),
    fetch('/api/v1/trafik/flow?county=' + code).then(r => r.json()),
  ]).then(([weatherRes, trafikRes, flowRes]) => {
    if (weatherRes.status === 'fulfilled') buildVader(weatherRes.value.data || []);
    const trafikData = trafikRes.status === 'fulfilled' ? trafikRes.value.data || [] : [];
    const flowData   = flowRes.status   === 'fulfilled' ? flowRes.value.data   || [] : [];
    if (trafikData.length || flowData.length) buildTrafik(trafikData, flowData);
  });

  function tempColor(t) {
    if (t == null) return 'var(--muted)';
    if (t >= 25)  return '#ff7b72';
    if (t >= 15)  return '#e3b341';
    if (t >= 5)   return '#3fb950';
    if (t >= 0)   return '#38b6ff';
    return '#b083f0';
  }

  function buildVader(stations) {
    if (!stations.length) return;
    const withTemp = stations.filter(s => s.temperature != null).sort((a, b) => b.temperature - a.temperature);
    if (!withTemp.length) return;

    document.getElementById('vaderSection').style.display = '';
    document.getElementById('vaderSub').textContent = withTemp.length + ' stationer · SMHI';

    const warmest = withTemp[0];
    const coldest = withTemp[withTemp.length - 1];
    const avg = withTemp.reduce((s, d) => s + d.temperature, 0) / withTemp.length;

    document.getElementById('vaderKpis').innerHTML =
      kpiCard('Varmast', fmtDec(warmest.temperature) + ' °C', esc(warmest.name || ''), tempColor(warmest.temperature)) +
      kpiCard('Kallast', fmtDec(coldest.temperature) + ' °C', esc(coldest.name || ''), tempColor(coldest.temperature)) +
      kpiCard('Snitt', fmtDec(avg) + ' °C', withTemp.length + ' aktiva stationer', tempColor(avg));

    document.getElementById('stationGrid').innerHTML = withTemp.map(s =>
      '<div class="station-item">' +
      '<span class="station-name" title="' + esc(s.name || '') + '">' + esc(s.name || '—') + '</span>' +
      '<span class="station-temp" style="color:' + tempColor(s.temperature) + '">' + fmtDec(s.temperature) + '°</span>' +
      '</div>'
    ).join('');
  }

  function kpiCard(label, val, sub, color) {
    return '<div class="kpi-card">' +
      '<div class="kpi-label">' + label + '</div>' +
      '<div class="kpi-val" style="color:' + (color || 'var(--ac)') + '">' + val + '</div>' +
      '<div class="kpi-sub">' + sub + '</div>' +
      '</div>';
  }

  function speedColor(kmh) {
    if (!kmh) return 'var(--dim)';
    if (kmh >= 80) return '#3fb950';
    if (kmh >= 50) return '#e3b341';
    if (kmh >= 30) return '#f0883e';
    return '#ff7b72';
  }

  function buildTrafik(conditions, flow) {
    document.getElementById('trafikSection').style.display = '';
    const el = document.getElementById('trafikSummary');
    let html = '';

    // Flow card — only available for ab/o/m in free tier
    const flowRow = flow.find(d => d.county === code);
    if (flowRow) {
      const spd = flowRow.avg_speed != null ? flowRow.avg_speed.toFixed(1) : '—';
      const flw = flowRow.avg_flow_per_lane != null ? Math.round(flowRow.avg_flow_per_lane).toLocaleString('sv-SE') : '—';
      const pct = flowRow.avg_speed ? Math.min(100, (flowRow.avg_speed / 110) * 100) : 0;
      const col = speedColor(flowRow.avg_speed);
      html += '<div class="trafik-flow-card">' +
        '<div class="tfc-label">Trafikflöde · realtidssensorer</div>' +
        '<div class="trafik-flow-stats">' +
        '<div><div class="tfc-val" style="color:' + col + '">' + esc(spd) + '<span style="font-size: .82rem;color:var(--muted)"> km/h</span></div><div class="tfc-label">Medelhastighet</div></div>' +
        '<div><div class="tfc-val" style="color:var(--ac)">' + esc(flw) + '</div><div class="tfc-label">Fordon/körfält/h</div></div>' +
        '</div>' +
        '<div class="speed-bar"><div class="speed-fill" style="width:' + pct + '%;background:' + col + '"></div></div>' +
        '<div style="font-family:\'DM Mono\',monospace;font-size:.64rem;color:var(--dim);margin-top:10px">' + (flowRow.measurements || 0).toLocaleString('sv-SE') + ' aktiva mätpunkter</div>' +
        '</div>';
    }

    // Road conditions card
    if (conditions.length) {
      const condCounts = {};
      conditions.forEach(d => { condCounts[d.site_type] = (condCounts[d.site_type] || 0) + 1; });
      const pills = Object.entries(condCounts)
        .sort((a, b) => b[1] - a[1])
        .map(([type, n]) => {
          const cls = type === 'Normalt' ? 'cond-1' : 'cond-other';
          return '<span class="cond-pill ' + cls + '">' + esc(type) + ' <strong>' + n + '</strong></span>';
        }).join('');

      html += '<div class="trafik-flow-card">' +
        '<div class="tfc-label">Vägförhållanden · ' + conditions.length + ' segment</div>' +
        '<div class="cond-pills" style="margin-top:14px">' + pills + '</div>' +
        '<div style="font-family:\'DM Mono\',monospace;font-size:.64rem;color:var(--dim);margin-top:12px">Källa: Trafikverket RoadCondition · <a href="/trafik" style="color:var(--ac);text-decoration:none">Alla trafikdata →</a></div>' +
        '</div>';
    }

    el.innerHTML = html || '<div style="color:var(--dim);font-family:\'DM Mono\',monospace;font-size: .82rem">Ingen trafikdata för detta län.</div>';
  }
}

/* ══════════════════════════════════════════════════════════════
   Länsanalys — ranking cards, radar chart, political sections
   ══════════════════════════════════════════════════════════════ */

const PARTY_COLORS = {
  S:'#E8112D', M:'#1B70AE', SD:'#DDCC00', C:'#009933',
  V:'#B5242D', KD:'#231977', L:'#006AB3', MP:'#83C31E', OVR:'#888888'
};
const PARTY_ORDER = ['S','SD','M','C','V','KD','L','MP','OVR'];

function rankColor(rank) {
  if (rank <= 5)  return 'rank-good';
  if (rank <= 14) return 'rank-mid';
  return 'rank-bad';
}

let radarChartObj   = null;
let politikChartObj = null;

function buildRankCards(rankings) {
  const me = rankings.find(r => r.code === code);
  if (!me) return;

  const m = me.metrics;
  const p = me.politics_2022;

  const cards = [
    { label:'Medianinkomst',   rank: m.income_rank,       val: m.income_median ? m.income_median.toFixed(0) + ' tkr' : '—' },
    { label:'Arbetslöshet',    rank: m.unemployment_rank,  val: m.unemployment_pct ? m.unemployment_pct.toFixed(1).replace('.',',') + ' %' : '—' },
    { label:'Meritvärde',      rank: m.school_grade_rank,  val: m.school_grade ? m.school_grade.toFixed(0) + ' poäng' : '—' },
    { label:'Livslängd',       rank: m.life_expectancy_rank, val: m.life_expectancy ? m.life_expectancy.toFixed(0) + ' år' : '—' },
    { label:'Brottslighet',    rank: m.crime_rank,         val: m.crime_per_100k ? Math.round(m.crime_per_100k).toLocaleString('sv-SE') + ' / 100k' : '—' },
    { label:'Socialbidrag', rank: m.welfare_rank, val: m.welfare_pct ? Math.round(m.welfare_pct).toLocaleString('sv-SE') + ' kr/inv' : '—' },
  ];

  const el = document.getElementById('rankCards');
  if (!el) return;
  el.innerHTML = cards.map(c => {
    const rankNum = c.rank || '—';
    const cls = c.rank ? rankColor(c.rank) : 'rank-mid';
    return '<div class="rank-card">' +
      '<div class="rc-label">' + esc(c.label) + '</div>' +
      '<div class="rc-rank ' + cls + '">' + rankNum +
        '<span class="rc-of"> av 21</span></div>' +
      '<div class="rc-val">' + esc(c.val) + '</div>' +
      '</div>';
  }).join('');

  // Build 2022 party bar
  const barEl = document.getElementById('politicsBar2022');
  if (barEl && p && p.parties) {
    const sorted = PARTY_ORDER
      .filter(pt => p.parties[pt] != null)
      .map(pt => ({ party: pt, pct: p.parties[pt] }))
      .sort((a, b) => b.pct - a.pct);
    const maxPct = sorted.length ? sorted[0].pct : 1;
    barEl.innerHTML = '<div class="party-bars">' +
      sorted.map(({ party, pct }) => {
        const col = PARTY_COLORS[party] || '#888';
        const width = (pct / maxPct * 100).toFixed(1);
        return '<div class="party-row">' +
          '<div class="p-name">' + esc(party) + '</div>' +
          '<div class="p-bar-wrap"><div class="p-bar" style="width:' + width + '%;background:' + col + '"></div></div>' +
          '<div class="p-pct">' + pct.toFixed(1).replace('.',',') + '%</div>' +
          '</div>';
      }).join('') +
      '</div>';

    const badge = document.getElementById('domPartyBadge');
    if (badge && p.dominant_party) {
      badge.innerHTML = 'Dominerande: <strong style="color:' + (PARTY_COLORS[p.dominant_party]||'#fff') + '">' +
        esc(p.dominant_party) + '</strong> ' + p.dominant_pct.toFixed(1).replace('.',',') + '%';
    }
  }

  // Radar chart — percentile score (21 - rank + 1) / 21
  const radarCtx = document.getElementById('radarChart');
  if (!radarCtx) return;
  const toScore = rank => (rank && rank > 0) ? Math.round(((22 - rank) / 21) * 100) : null;
  const radarData = [
    toScore(m.income_rank),
    toScore(m.unemployment_rank),
    toScore(m.school_grade_rank),
    toScore(m.life_expectancy_rank),
    toScore(m.crime_rank),
    toScore(m.welfare_rank),
  ];

  if (radarChartObj) radarChartObj.destroy();
  radarChartObj = new Chart(radarCtx, {
    type: 'radar',
    data: {
      labels: ['Inkomst','Sysselsättning','Utbildning','Hälsa','Trygghet','Socialbidrag'],
      datasets: [{
        data: radarData.map(v => v ?? 0),
        backgroundColor: accentColor + '33',
        borderColor: accentColor,
        borderWidth: 2,
        pointBackgroundColor: radarData.map(v => v != null ? accentColor : 'transparent'),
        pointRadius: 4,
      }]
    },
    options: {
      responsive: true, maintainAspectRatio: true,
      scales: {
        r: {
          min: 0, max: 100,
          ticks: { display: false, stepSize: 25 },
          grid: { color: 'rgba(42,63,95,.5)' },
          angleLines: { color: 'rgba(42,63,95,.5)' },
          pointLabels: {
            color: '#8da6c0',
            font: { family: "'DM Mono','monospace'", size: 11 }
          }
        }
      },
      plugins: { legend: { display: false } }
    }
  });
}

function buildProfil(prof) {
  if (!prof || !prof.election_history || !prof.election_history.length) return;

  const ctx = document.getElementById('politikChart');
  if (!ctx) return;

  const years = prof.election_history.map(e => e.year);
  const parties = ['S','SD','M','C','V','KD','L','MP'];

  const datasets = parties.map(party => {
    const data = prof.election_history.map(e => e[party] != null ? +e[party].toFixed(1) : null);
    if (data.every(v => v == null || v < 0.5)) return null;
    return {
      label: party,
      data: data,
      borderColor: PARTY_COLORS[party] || '#888',
      backgroundColor: (PARTY_COLORS[party] || '#888') + '22',
      borderWidth: 2,
      pointRadius: 3,
      tension: 0.3,
      spanGaps: true,
    };
  }).filter(Boolean);

  if (politikChartObj) politikChartObj.destroy();
  politikChartObj = new Chart(ctx, {
    type: 'line',
    data: { labels: years, datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      scales: {
        x: { grid: { color: 'rgba(42,63,95,.4)' }, ticks: { color: '#6b82a0' } },
        y: {
          grid: { color: 'rgba(42,63,95,.4)' }, ticks: { color: '#6b82a0' },
          title: { display: true, text: 'Röstandel %', color: '#4d647d' }
        }
      },
      plugins: {
        legend: {
          labels: { color: '#8da6c0', font: { family: "'DM Mono','monospace'", size: 11 }, boxWidth: 12 }
        },
        tooltip: { mode: 'index', intersect: false }
      }
    }
  });
}
