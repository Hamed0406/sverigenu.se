(function () {
  'use strict';

  var COUNTY_COLORS = {
    ab: '#4fa3e8', o: '#2dd4a0', m: '#7c6cff', c: '#79c0ff',
    e: '#d29922', f: '#56d364', n: '#ff7b72', bd: '#38b6ff',
    x: '#bc8cff', y: '#f78166', z: '#3fb950', w: '#e3b341',
    d: '#6cb6ff', g: '#57ab5a', h: '#e0823d', i: '#b083f0',
    k: '#ec8e7b', s: '#54aeff', t: '#6fdd8b', u: '#daaa3f', ac: '#8ddb8c'
  };
  var DEFAULT_ON = ['ab', 'o', 'm', 'bd'];

  var fmt = new Intl.NumberFormat('sv-SE');

  function fmtCompact(n) {
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace('.', ',') + ' M';
    if (n >= 1e3) return Math.round(n / 1e3) + ' k';
    return String(n);
  }

  /* Chart.js dark defaults */
  Chart.defaults.color = '#6b82a0';
  Chart.defaults.borderColor = 'rgba(27,41,64,.6)';
  Chart.defaults.font.family = "'DM Mono', monospace";
  Chart.defaults.font.size = 10;

  var byCounty = {};   // code -> [{year, population}]
  var names = {};      // code -> name (from lan/summary)
  var selected = DEFAULT_ON.slice();
  var chart = null;

  function countUp(el, target, dur, formatter) {
    var start = null;
    function frame(ts) {
      if (!start) start = ts;
      var t = Math.min((ts - start) / dur, 1);
      var eased = 1 - Math.pow(1 - t, 3);
      el.textContent = formatter(Math.round(target * eased));
      if (t < 1) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }

  function buildChart() {
    var ctx = document.getElementById('ts-chart');
    var datasets = selected.map(function (code) {
      var series = byCounty[code] || [];
      return {
        label: names[code] || code.toUpperCase(),
        data: series.map(function (r) { return { x: r.year, y: r.population }; }),
        borderColor: COUNTY_COLORS[code] || '#4fa3e8',
        backgroundColor: 'transparent',
        borderWidth: 2,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHoverBackgroundColor: COUNTY_COLORS[code],
        tension: .25
      };
    });

    if (chart) { chart.destroy(); }
    chart = new Chart(ctx, {
      type: 'line',
      data: { datasets: datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'nearest', axis: 'x', intersect: false },
        plugins: {
          legend: {
            labels: { usePointStyle: true, pointStyle: 'line', boxWidth: 24, padding: 16,
                      font: { family: "'DM Sans', sans-serif", size: 11 } }
          },
          tooltip: {
            backgroundColor: '#0f1726',
            borderColor: '#2a3f5f',
            borderWidth: 1,
            titleFont: { family: "'Fraunces', serif", size: 13 },
            bodyFont: { family: "'DM Mono', monospace", size: 11 },
            padding: 12,
            callbacks: {
              label: function (item) {
                return ' ' + item.dataset.label + ': ' + fmt.format(item.parsed.y);
              }
            }
          }
        },
        scales: {
          x: { type: 'linear', min: 1968, max: 2024,
               ticks: { stepSize: 8, callback: function (v) { return v; } },
               grid: { color: 'rgba(27,41,64,.35)' } },
          y: { ticks: { callback: function (v) { return fmtCompact(v); } },
               grid: { color: 'rgba(27,41,64,.35)' } }
        }
      }
    });
  }

  function buildPicker() {
    var picker = document.getElementById('picker');
    picker.innerHTML = '';
    Object.keys(names).forEach(function (code) {
      var b = document.createElement('button');
      b.className = 'pick' + (selected.indexOf(code) !== -1 ? ' on' : '');
      b.textContent = names[code];
      b.style.setProperty('--pc', COUNTY_COLORS[code] || '#4fa3e8');
      b.addEventListener('click', function () {
        var i = selected.indexOf(code);
        if (i !== -1) {
          selected.splice(i, 1);
        } else {
          if (selected.length >= 6) selected.shift(); // oldest pick drops off
          selected.push(code);
        }
        buildPicker();
        buildChart();
      });
      picker.appendChild(b);
    });
  }

  function buildGrowth() {
    var rows = Object.keys(byCounty).map(function (code) {
      var series = byCounty[code];
      var y2000 = null, last = null;
      series.forEach(function (r) {
        if (r.year === 2000) y2000 = r.population;
        if (!last || r.year > last.year) last = r;
      });
      if (!y2000 || !last) return null;
      return { code: code, name: names[code] || code, pct: (last.population - y2000) / y2000 * 100 };
    }).filter(Boolean).sort(function (a, b) { return b.pct - a.pct; });

    var maxAbs = Math.max.apply(null, rows.map(function (r) { return Math.abs(r.pct); }));
    var grid = document.getElementById('growth');
    rows.forEach(function (r, i) {
      var row = document.createElement('div');
      row.className = 'growth-row';
      var cls = r.pct >= 0 ? 'pos' : 'neg';
      row.innerHTML =
        '<div class="growth-name">' + r.name + '</div>' +
        '<div class="growth-track"><div class="growth-fill ' + (r.pct < 0 ? 'neg' : '') + '" data-w="' + (Math.abs(r.pct) / maxAbs * 100) + '"></div></div>' +
        '<div class="growth-val ' + cls + '">' + (r.pct >= 0 ? '+' : '') + r.pct.toFixed(1).replace('.', ',') + '&thinsp;%</div>';
      grid.appendChild(row);
    });

    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (!en.isIntersecting) return;
        var kids = Array.prototype.slice.call(grid.children);
        kids.forEach(function (row, i) {
          setTimeout(function () {
            row.classList.add('in');
            var fill = row.querySelector('.growth-fill');
            fill.style.width = fill.dataset.w + '%';
          }, i * 45);
        });
        io.disconnect();
      });
    }, { threshold: .2 });
    io.observe(grid);
  }

  function buildMuniTables(rows) {
    rows.sort(function (a, b) { return b.value - a.value; });
    var top = rows.slice(0, 10);
    var bottom = rows.slice(-10).reverse();

    function render(tbody, list, startRank, reverseRank) {
      list.forEach(function (m, i) {
        var tr = document.createElement('tr');
        var rank = reverseRank ? rows.length - i : startRank + i;
        tr.innerHTML =
          '<td class="rank">' + rank + '</td>' +
          '<td><span class="mname">' + m.name + '</span> <span class="mcounty">' + (names[m.county] || m.county) + '</span></td>' +
          '<td class="mval">' + fmt.format(Math.round(m.value)) + '</td>';
        tbody.appendChild(tr);
      });
    }
    render(document.getElementById('top-munis'), top, 1, false);
    render(document.getElementById('bottom-munis'), bottom, 0, true);
  }

  /* ── Load everything ─────────────────────────────── */
  Promise.all([
    fetch('/api/v1/lan/summary').then(function (r) { return r.json(); }),
    fetch('/api/v1/befolkning').then(function (r) { return r.json(); }),
    fetch('/api/v1/kolada?kpi=N01951').then(function (r) { return r.json(); })
  ]).then(function (res) {
    var summary = res[0].data || [];
    var series = res[1].data || [];
    var munis = (res[2].data || []).map(function (m) {
      return { name: m.name, county: m.county, value: m.value };
    });

    summary.forEach(function (r) { names[r.code] = r.name; });

    series.forEach(function (r) {
      (byCounty[r.county] = byCounty[r.county] || []).push({ year: r.year, population: r.population });
    });
    Object.keys(byCounty).forEach(function (c) {
      byCounty[c].sort(function (a, b) { return a.year - b.year; });
    });

    /* hero band */
    var total = 0, latestYear = 0, biggest = null;
    summary.forEach(function (r) {
      if (r.population) {
        total += r.population;
        if (r.population_year > latestYear) latestYear = r.population_year;
        if (!biggest || r.population > biggest.population) biggest = r;
      }
    });
    countUp(document.getElementById('total-pop'), total, 1600, function (n) { return fmt.format(n); });
    document.getElementById('total-year').textContent = 'folkbokförda ' + latestYear + '-12-31';
    if (biggest) {
      document.getElementById('biggest-lan').textContent = biggest.name;
      document.getElementById('biggest-pop').textContent = fmt.format(biggest.population) + ' inv';
    }

    var fastest = null;
    Object.keys(byCounty).forEach(function (code) {
      var s = byCounty[code], y2000 = null, last = null;
      s.forEach(function (r) {
        if (r.year === 2000) y2000 = r.population;
        if (!last || r.year > last.year) last = r;
      });
      if (!y2000) return;
      var pct = (last.population - y2000) / y2000 * 100;
      if (!fastest || pct > fastest.pct) fastest = { code: code, pct: pct };
    });
    if (fastest) {
      document.getElementById('fastest-lan').textContent = names[fastest.code];
      document.getElementById('fastest-pct').textContent = '+' + fastest.pct.toFixed(1).replace('.', ',') + ' % sedan 2000';
    }

    buildPicker();
    buildChart();
    buildGrowth();
    buildMuniTables(munis);
  }).catch(function (err) {
    document.getElementById('chart-empty').hidden = false;
    document.getElementById('total-pop').textContent = '—';
    console.error('API load failed:', err);
  });
})();

/* ─── Förlossningar & nyfödda ───────────────────────────────── */
(async function() {
  try {
    const r = await fetch('/api/v1/befolkning/forlossningar');
    if (!r.ok) throw new Error(r.status);
    const rows = (await r.json()).data || [];

    const latestYear = Math.max(...rows.map(d=>d.year));
    document.getElementById('birthsSub').textContent =
      `Socialstyrelsen · ${latestYear} · riket och landsting`;

    // Hero cards — V35 (births), V18 (cesarian), V41 (low birth weight), V10 (smoking).
    const heroVars = [
      {id:'V35', label:'Födda barn', fmt: v=>`${v.toLocaleString('sv-SE')}`},
      {id:'V18', label:'Kejsarsnitt', fmt: v=>`${v.toLocaleString('sv-SE')}`},
      {id:'V41', label:'Undervikt vid födseln', fmt: v=>`${v.toLocaleString('sv-SE')}`},
      {id:'V10', label:'Röker i tidig graviditet', fmt: v=>`${v.toLocaleString('sv-SE')}`},
    ];
    const heroGrid = document.getElementById('birthsHero');
    heroGrid.innerHTML = heroVars.map(hv => {
      const row = rows.find(r=>r.variabel_id===hv.id && r.region_id===0 && r.year===latestYear);
      const val = row?.value;
      return `<div class="kpi-card"><div class="kpi-label">${hv.label}</div>
        <div class="kpi-val">${val!=null ? hv.fmt(val) : '–'}</div>
        <div class="kpi-sub">${latestYear} · Riket</div></div>`;
    }).join('');

    // Line chart: Födda barn (V35) riket over years.
    const v35Riket = rows.filter(r=>r.variabel_id==='V35' && r.region_id===0)
      .sort((a,b)=>a.year-b.year);
    new Chart(document.getElementById('birthsLineChart'), {
      type:'line',
      data:{
        labels: v35Riket.map(d=>d.year),
        datasets:[{
          label:'Födda barn',
          data: v35Riket.map(d=>d.value),
          borderColor:'#4fa3e8', backgroundColor:'#4fa3e822',
          tension:0.3, pointRadius:3, fill:true,
        }]
      },
      options:{
        responsive:true,
        plugins:{ legend:{display:false} },
        scales:{
          x:{ ticks:{color:'#aaa'}, grid:{color:'#333'} },
          y:{ ticks:{color:'#aaa'}, grid:{color:'#333'},
              title:{display:true,text:'Antal',color:'#aaa'} }
        }
      }
    });

    // Bar chart: Kejsarsnitt % (V18 / V17) per region, latest year, excluding Riket.
    const cesRows = rows.filter(r=>r.year===latestYear && r.region_id!==0);
    const totalByRegion = {};
    rows.filter(r=>r.variabel_id==='V17' && r.year===latestYear && r.region_id!==0)
      .forEach(r=>{ if(r.value) totalByRegion[r.region_id]=r.value; });
    const cesData = rows.filter(r=>r.variabel_id==='V18' && r.year===latestYear && r.region_id!==0)
      .map(r=>({
        name: r.region_name,
        pct: totalByRegion[r.region_id] ? (r.value/totalByRegion[r.region_id]*100) : null
      }))
      .filter(d=>d.pct!=null)
      .sort((a,b)=>b.pct-a.pct);
    new Chart(document.getElementById('birthsBarChart'), {
      type:'bar',
      data:{
        labels: cesData.map(d=>d.name),
        datasets:[{
          label:'Kejsarsnitt %',
          data: cesData.map(d=>d.pct),
          backgroundColor:'#ab47bcaa', borderColor:'#ab47bc', borderWidth:1,
        }]
      },
      options:{
        indexAxis:'y', responsive:true,
        plugins:{ legend:{display:false} },
        scales:{
          x:{ ticks:{color:'#aaa'}, grid:{color:'#333'},
              title:{display:true,text:'%',color:'#aaa'} },
          y:{ ticks:{color:'#aaa',font:{size:10}}, grid:{color:'#222'} }
        }
      }
    });
  } catch(e) { console.error('births fetch failed', e); }
})();
