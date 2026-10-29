(function () {
  'use strict';

  function esc(s) {
    return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }
  function safeRiksdagenPhoto(url) {
    if (!url) return '';
    var s = String(url);
    return (s.startsWith('https://data.riksdagen.se/') || s.startsWith('https://www.riksdagen.se/')) ? s : '';
  }

  // Party colours — approximate Swedish political tradition
  var PARTY_COLORS = {
    'S':  '#e8112d', // socialdemokraterna — red
    'SD': '#DDCB00', // sverigedemokraterna — yellow
    'M':  '#52BDEC', // moderaterna — blue
    'C':  '#009933', // centerpartiet — green
    'V':  '#DA291C', // vänsterpartiet — darker red
    'KD': '#231977', // kristdemokraterna — dark blue
    'L':  '#006AB3', // liberalerna — blue
    'MP': '#83CF39', // miljöpartiet — lime green
    '-':  '#6b82a0'  // unknown/independent
  };

  Chart.defaults.color = '#6b82a0';
  Chart.defaults.borderColor = 'rgba(27,41,64,.6)';
  Chart.defaults.font.family = "'DM Mono', monospace";
  Chart.defaults.font.size = 10;

  var allMembers = [];
  var activePartyFilter = 'ALL';
  var activeValkrets = '';

  function buildSeats(seats) {
    var total = seats.reduce(function (s, r) { return s + r.seats; }, 0);
    var max = seats[0] ? seats[0].seats : 1;
    var grid = document.getElementById('seats-grid');

    seats.forEach(function (r, i) {
      var row = document.createElement('div');
      row.className = 'seat-row';
      var color = PARTY_COLORS[r.party] || '#4fa3e8';
      var wPct = total > 0 ? (r.women / r.seats * 100).toFixed(0) : 0;
      var w = (r.seats / max * 100).toFixed(1);
      row.innerHTML =
        '<div class="seat-party" style="color:' + color + '">' + esc(r.party) + '</div>' +
        '<div class="seat-track">' +
          '<div class="seat-fill" data-w="' + w + '" style="background:' + color + '">' +
            '<span class="seat-gender" style="color:rgba(255,255,255,.7)">' + wPct + '% ♀</span>' +
          '</div>' +
        '</div>' +
        '<div class="seat-num">' + r.seats + ' mandat</div>';
      grid.appendChild(row);
      // staggered reveal
      setTimeout(function () {
        row.classList.add('in');
        row.querySelector('.seat-fill').style.width = row.querySelector('.seat-fill').dataset.w + '%';
      }, i * 60 + 100);
    });

    // Donut chart
    var ctx = document.getElementById('donut-chart');
    new Chart(ctx, {
      type: 'doughnut',
      data: {
        labels: seats.map(function (r) { return r.party + ' (' + r.seats + ')'; }),
        datasets: [{
          data: seats.map(function (r) { return r.seats; }),
          backgroundColor: seats.map(function (r) { return PARTY_COLORS[r.party] || '#4fa3e8'; }),
          borderColor: '#0f1726',
          borderWidth: 2,
          hoverOffset: 8
        }]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        cutout: '62%',
        plugins: {
          legend: { position: 'right', labels: { font: { family: "'DM Sans', sans-serif", size: 12 }, padding: 14, usePointStyle: true } },
          tooltip: {
            backgroundColor: '#0f1726', borderColor: '#2a3f5f', borderWidth: 1,
            titleFont: { family: "'Fraunces', serif", size: 13 },
            bodyFont: { family: "'DM Mono', monospace", size: 11 },
            padding: 12
          }
        }
      }
    });
  }

  function buildFilterBar(seats) {
    var bar = document.getElementById('filter-bar');
    var all = document.createElement('button');
    all.className = 'filter-btn on';
    all.textContent = 'Alla partier';
    all.style.setProperty('--pc', 'var(--accent)');
    all.addEventListener('click', function () { setPartyFilter('ALL'); });
    bar.appendChild(all);
    seats.forEach(function (r) {
      if (r.party === '-') return;
      var btn = document.createElement('button');
      var color = PARTY_COLORS[r.party] || '#4fa3e8';
      btn.className = 'filter-btn';
      btn.textContent = r.party + ' · ' + r.seats;
      btn.style.setProperty('--pc', color);
      btn.dataset.party = r.party;
      btn.addEventListener('click', function () { setPartyFilter(r.party); });
      bar.appendChild(btn);
    });
  }

  function buildValkretsSelect() {
    var sel = document.getElementById('vk-select');
    var vkList = [];
    allMembers.forEach(function (m) { if (m.valkrets && vkList.indexOf(m.valkrets) === -1) vkList.push(m.valkrets); });
    vkList.sort();
    vkList.forEach(function (vk) {
      var opt = document.createElement('option');
      opt.value = vk;
      opt.textContent = vk;
      sel.appendChild(opt);
    });
    sel.addEventListener('change', function () {
      activeValkrets = sel.value;
      updateValkretsSummary();
      renderMPs();
    });
  }

  function updateValkretsSummary() {
    var box = document.getElementById('vk-summary');
    if (!activeValkrets) { box.classList.remove('visible'); return; }
    var members = allMembers.filter(function (m) { return m.valkrets === activeValkrets; });
    var total = members.length;
    var women = members.filter(function (m) { return m.gender === 'kvinna'; }).length;
    var womenPct = total > 0 ? Math.round(women / total * 100) : 0;

    // Party breakdown
    var byParty = {};
    members.forEach(function (m) { byParty[m.party] = (byParty[m.party] || 0) + 1; });
    var parties = Object.keys(byParty).sort(function (a, b) { return byParty[b] - byParty[a]; });

    document.getElementById('vk-summary-title').innerHTML = '<em>' + esc(activeValkrets) + '</em>';
    document.getElementById('vk-stat-row').innerHTML =
      '<div class="vk-stat"><div class="vk-stat-num">' + total + '</div><div class="vk-stat-lbl">Ledamöter</div></div>' +
      '<div class="vk-stat"><div class="vk-stat-num">' + parties.length + '</div><div class="vk-stat-lbl">Partier</div></div>' +
      '<div class="vk-stat"><div class="vk-stat-num">' + womenPct + ' %</div><div class="vk-stat-lbl">Kvinnor (' + women + '/' + total + ')</div></div>';

    var chipsHTML = parties.map(function (p) {
      var color = PARTY_COLORS[p] || '#4fa3e8';
      var n = byParty[p];
      return '<span class="vk-chip" style="background:color-mix(in srgb,' + color + ' 18%,transparent);color:' + color + ';border:1px solid color-mix(in srgb,' + color + ' 35%,transparent)">' +
        esc(p) + ' · ' + n + ' st</span>';
    }).join('');
    document.getElementById('vk-party-chips').innerHTML = chipsHTML;
    box.classList.add('visible');
  }

  function setPartyFilter(party) {
    activePartyFilter = party;
    document.querySelectorAll('#filter-bar .filter-btn').forEach(function (b) {
      b.classList.remove('on');
      if ((party === 'ALL' && b.textContent === 'Alla partier') || b.dataset.party === party) b.classList.add('on');
    });
    renderMPs();
  }

  function renderMPs() {
    var filtered = allMembers.filter(function (m) {
      var partyOk = activePartyFilter === 'ALL' || m.party === activePartyFilter;
      var vkOk    = !activeValkrets || m.valkrets === activeValkrets;
      return partyOk && vkOk;
    });
    var grid = document.getElementById('mp-grid');
    var count = document.getElementById('mp-count');
    count.textContent = filtered.length + ' ledamöter';
    grid.innerHTML = '';
    filtered.forEach(function (m) {
      var card = document.createElement('div');
      card.className = 'mp-card';
      var color = PARTY_COLORS[m.party] || '#4fa3e8';
      var initials = esc((m.first_name.charAt(0) + m.last_name.charAt(0)).toUpperCase());
      var safePhoto = safeRiksdagenPhoto(m.photo_url);
      var photoEl = safePhoto
        ? '<img class="mp-photo" src="' + esc(safePhoto) + '" alt="' + esc(m.first_name) + ' ' + esc(m.last_name) + '" loading="lazy" onerror="this.style.display=\'none\';this.nextElementSibling.style.display=\'flex\'">' +
          '<div class="mp-photo-placeholder" style="display:none">' + initials + '</div>'
        : '<div class="mp-photo-placeholder">' + initials + '</div>';
      card.innerHTML =
        photoEl +
        '<div class="mp-name">' + esc(m.first_name) + ' ' + esc(m.last_name) + '</div>' +
        '<div class="mp-valkrets">' + esc(m.valkrets) + '</div>' +
        '<span class="mp-badge" style="background:color-mix(in srgb,' + color + ' 18%,transparent);color:' + color + ';border:1px solid color-mix(in srgb,' + color + ' 35%,transparent)">' + esc(m.party) + '</span>';
      grid.appendChild(card);
    });
  }

  function buildRegionTable() {
    // Build per-valkrets stats from allMembers
    var byVk = {};
    allMembers.forEach(function (m) {
      if (!byVk[m.valkrets]) byVk[m.valkrets] = { total: 0, women: 0, parties: {} };
      byVk[m.valkrets].total++;
      if (m.gender === 'kvinna') byVk[m.valkrets].women++;
      byVk[m.valkrets].parties[m.party] = (byVk[m.valkrets].parties[m.party] || 0) + 1;
    });

    var sorted = Object.keys(byVk).sort(function (a, b) { return byVk[b].total - byVk[a].total; });
    var tbody = document.getElementById('region-tbody');

    sorted.forEach(function (vk) {
      var r = byVk[vk];
      var wPct = Math.round(r.women / r.total * 100);
      var parties = Object.keys(r.parties).sort(function (a, b) { return r.parties[b] - r.parties[a]; });
      var chipsHTML = parties.map(function (p) {
        var color = PARTY_COLORS[p] || '#4fa3e8';
        return '<span class="rt-chip" style="background:color-mix(in srgb,' + color + ' 15%,transparent);color:' + color + ';border:1px solid color-mix(in srgb,' + color + ' 30%,transparent)">' +
          esc(p) + '&thinsp;' + r.parties[p] + '</span>';
      }).join('');
      var tr = document.createElement('tr');
      tr.style.cursor = 'pointer';
      tr.title = 'Filtrera på ' + esc(vk);
      tr.innerHTML =
        '<td class="rt-vk">' + esc(vk) + '</td>' +
        '<td class="rt-n">' + r.total + '</td>' +
        '<td class="rt-w">' + wPct + ' %</td>' +
        '<td><div class="rt-parties">' + chipsHTML + '</div></td>';
      // clicking a row sets the valkrets filter
      tr.addEventListener('click', function () {
        var sel = document.getElementById('vk-select');
        sel.value = vk;
        activeValkrets = vk;
        setPartyFilter('ALL');
        updateValkretsSummary();
        renderMPs();
        document.getElementById('mp-grid').scrollIntoView({ behavior: 'smooth', block: 'start' });
      });
      tbody.appendChild(tr);
    });
  }

  function buildVotes(votes) {
    var list = document.getElementById('vote-list');
    if (!votes.length) {
      list.innerHTML = '<p style="color:var(--dim);font-family:\'DM Mono\',monospace;font-size: .82rem">Inga omröstningar hittades.</p>';
      return;
    }
    // Main parties order
    var PARTY_ORDER = ['S', 'M', 'SD', 'V', 'C', 'MP', 'KD', 'L'];

    votes.forEach(function (v) {
      var card = document.createElement('div');
      card.className = 'vote-card';

      var title = esc(v.title || v.beteckning || v.votering_id.slice(0, 12) + '…');
      var datum = v.datum || '';

      // Aggregate totals
      var totJa = 0, totNej = 0, totAvst = 0, totFranv = 0;
      v.parties.forEach(function (p) { totJa += p.ja; totNej += p.nej; totAvst += p.avstar; totFranv += p.franvarande; });
      var total = totJa + totNej + totAvst + totFranv || 1;

      // Sort parties by order, skip independents and empty rows
      var sortedParties = PARTY_ORDER.map(function (pn) {
        return v.parties.find(function (p) { return p.party === pn; });
      }).filter(Boolean).filter(function (p) { return p.ja + p.nej + p.avstar + p.franvarande > 0; });

      var barsHTML = sortedParties.map(function (p) {
        var rowTotal = p.ja + p.nej + p.avstar + p.franvarande || 1;
        var wJa   = (p.ja          / rowTotal * 100).toFixed(1);
        var wNej  = (p.nej         / rowTotal * 100).toFixed(1);
        var wAvst = (p.avstar      / rowTotal * 100).toFixed(1);
        var wFrv  = (p.franvarande / rowTotal * 100).toFixed(1);
        var color = PARTY_COLORS[p.party] || '#4fa3e8';
        return '<div class="vbar-row">' +
          '<div class="vbar-party" style="color:' + color + '">' + esc(p.party) + '</div>' +
          '<div class="vbar-track">' +
            '<div class="vbar-ja"   style="width:' + wJa   + '%"></div>' +
            '<div class="vbar-nej"  style="width:' + wNej  + '%"></div>' +
            '<div class="vbar-avst" style="width:' + wAvst + '%"></div>' +
            '<div class="vbar-franv" style="width:' + wFrv + '%"></div>' +
          '</div>' +
          '<div class="vbar-nums"><span class="j">' + p.ja + ' ja</span><span class="n">' + p.nej + ' nej</span></div>' +
        '</div>';
      }).join('');

      var outcome = totJa > totNej ? '<span style="color:var(--green)">Bifall</span>' : totNej > totJa ? '<span style="color:var(--red)">Avslag</span>' : '<span style="color:var(--muted)">Lika</span>';

      card.innerHTML =
        '<div class="vote-header">' +
          '<div class="vote-title">' + title + '</div>' +
          '<div class="vote-meta">' + (datum ? esc(datum) + '<br>' : '') + esc(v.rm) + '<br>' + outcome + '</div>' +
        '</div>' +
        '<div class="vote-legend">' +
          '<span><span class="sw" style="background:var(--green)"></span>Ja (' + totJa + ')</span>' +
          '<span><span class="sw" style="background:var(--red)"></span>Nej (' + totNej + ')</span>' +
          '<span><span class="sw" style="background:#6b82a0"></span>Avstår (' + totAvst + ')</span>' +
          '<span><span class="sw" style="background:rgba(61,82,112,.4)"></span>Frånv. (' + totFranv + ')</span>' +
        '</div>' +
        '<div class="vote-bars">' + barsHTML + '</div>';
      list.appendChild(card);
    });
  }

  // ── Load all data ─────────────────────────────────
  Promise.all([
    fetch('/api/v1/riksdagen/seats').then(function (r) { return r.json(); }),
    fetch('/api/v1/riksdagen/members').then(function (r) { return r.json(); }),
    fetch('/api/v1/riksdagen/votes').then(function (r) { return r.json(); })
  ]).then(function (res) {
    var seats   = (res[0].data || []).filter(function (r) { return r.party !== '-'; });
    var members = res[1].data || [];
    var votes   = res[2].data || [];

    allMembers = members.filter(function (m) { return m.party !== '-'; });

    // Hero stats
    var totalSeats  = seats.reduce(function (s, r) { return s + r.seats; }, 0);
    var totalWomen  = seats.reduce(function (s, r) { return s + r.women; }, 0);
    var womenPct    = totalSeats > 0 ? Math.round(totalWomen / totalSeats * 100) : 0;
    document.getElementById('total-members').textContent = totalSeats;
    document.getElementById('total-parties').textContent = seats.length;
    document.getElementById('women-pct').textContent     = womenPct + ' %';
    document.getElementById('women-sub').textContent     = totalWomen + ' av ' + totalSeats;
    document.getElementById('total-votes').textContent   = votes.length;

    buildSeats(seats);
    buildFilterBar(seats);
    buildValkretsSelect();
    renderMPs();
    buildRegionTable();
    buildVotes(votes);
  }).catch(function (err) {
    console.error('Riksdagen API load failed:', err);
  });

  // ── Upcoming calendar (separate fetch — page works without it) ──
  var KAL_TYP = {
    'utskottsmöte': 'Utskott', 'kammakt': 'Kammaren', 'sammanträde': 'Sammanträde',
    'debatt': 'Debatt', 'votering': 'Votering', 'samtr': 'Sammanträde',
    'seminarium': 'Seminarium', 'besök': 'Besök'
  };

  fetch('/api/v1/riksdagen/kalender').then(function (r) { return r.json(); }).then(function (res) {
    var list = document.getElementById('kal-list');
    if (!list) return;
    var events = res.data || [];
    if (!events.length) {
      list.innerHTML = '<p class="chart-note">Inga inplanerade händelser de närmaste veckorna (kalendern är gles under sommaruppehållet).</p>';
      return;
    }
    list.innerHTML = '';
    var days = ['sön', 'mån', 'tis', 'ons', 'tor', 'fre', 'lör'];
    events.slice(0, 25).forEach(function (ev) {
      var d = new Date(ev.datum.replace(' ', 'T'));
      var when = isNaN(d) ? ev.datum :
        days[d.getDay()] + ' ' + d.getDate() + '/' + (d.getMonth() + 1) +
        (d.getHours() || d.getMinutes() ? ' ' + String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0') : '');
      var item = document.createElement('div');
      item.className = 'kal-item';
      var title = document.createElement('div');
      title.className = 'kal-title';
      title.textContent = ev.titel;
      var body = document.createElement('div');
      body.className = 'kal-body';
      body.appendChild(title);
      // Skip riksdagen's internal short codes (bp, ss, ...) — only show
      // human-readable subtype/location strings.
      var metaTxt = [ev.subtyp, ev.plats].filter(function (s) {
        return s && s.length > 3;
      }).join(' · ');
      if (metaTxt) {
        var meta = document.createElement('div');
        meta.className = 'kal-meta';
        meta.textContent = metaTxt;
        body.appendChild(meta);
      }
      var date = document.createElement('span');
      date.className = 'kal-date';
      date.textContent = when;
      var typ = document.createElement('span');
      typ.className = 'kal-typ';
      typ.textContent = KAL_TYP[ev.typ] || ev.typ;
      item.appendChild(date);
      item.appendChild(body);
      item.appendChild(typ);
      list.appendChild(item);
    });
  }).catch(function (err) {
    console.error('Riksdagen kalender load failed:', err);
  });
})();
