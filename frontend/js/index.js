(function () {
  'use strict';

  /* ── Map: choropleth tint + staggered reveal ───────── */
  var paths = Array.prototype.slice.call(document.querySelectorAll('.lan'));
  var maxPop = 2425000;

  function popOf(p) {
    return parseInt(p.dataset.pop.replace(/\s/g, ''), 10) || 0;
  }

  /* sort north→south by path index for the reveal sweep */
  paths.forEach(function (p, i) {
    var t = Math.sqrt(popOf(p) / maxPop);          /* sqrt = perceptual ramp */
    var l = 10 + t * 14;                            /* lightness 10%..24% */
    p.style.fill = 'hsl(214 45% ' + l.toFixed(1) + '%)';
    p.style.animationDelay = (0.25 + i * 0.045) + 's';
  });

  /* ── Live data from the backend ────────────────────── */
  /* Enriches tooltips with current temperature and police event counts.
     Fails silently — the page works standalone when no backend is running. */
  var live = {};
  try {
    fetch('/api/v1/lan/summary')
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (!d || !d.data) return;
        d.data.forEach(function (row) { live[row.code] = row; });
      })
      .catch(function () {});

    // Live ekonomi stats for cards
    fetch('/api/v1/riksbank?series=policy_rate&from=2026-01-01')
      .then(function(r){ return r.ok ? r.json() : null; })
      .then(function(d){
        if (!d || !d.data || !d.data.length) return;
        var last = d.data[d.data.length - 1];
        var el = document.getElementById('idx-repo');
        if (el) el.textContent = last.value.toFixed(2);
      }).catch(function(){});

    fetch('/api/v1/elpris?area=SE3')
      .then(function(r){ return r.ok ? r.json() : null; })
      .then(function(d){
        if (!d || !d.data || !d.data.length) return;
        var now = new Date();
        var nowMs = now.getTime();
        var hour = d.data.find(function(x){
          var tStart = new Date(x.time_start).getTime();
          return nowMs >= tStart && nowMs < tStart + 15 * 60 * 1000;
        });
        if (!hour) return;
        var el = document.getElementById('idx-el');
        if (el) el.textContent = (hour.sek_per_kwh * 100).toFixed(0);
      }).catch(function(){});

    fetch('/api/v1/arbetsmarknad/jobs')
      .then(function(r){ return r.ok ? r.json() : null; })
      .then(function(d){
        if (!d || !d.data || !d.data.length) return;
        var total = d.data.reduce(function(s, r){ return s + r.total_ads; }, 0);
        var el = document.getElementById('idx-jobs');
        if (el) el.textContent = new Intl.NumberFormat('sv-SE').format(total);
      }).catch(function(){});

    fetch('/api/v1/trafik/flow?county=ab')
      .then(function(r){ return r.ok ? r.json() : null; })
      .then(function(d){
        if (!d || !d.data || !d.data.length) return;
        var el = document.getElementById('idx-trafik-speed');
        if (el && d.data[0].avg_speed != null) el.textContent = d.data[0].avg_speed.toFixed(0);
      }).catch(function(){});
  } catch (e) { /* file:// or very old browser */ }

  /* ── Tooltip ───────────────────────────────────────── */
  var tooltip  = document.getElementById('tooltip');
  var ttName   = document.getElementById('tt-name');
  var ttCode   = document.getElementById('tt-code');
  var ttPop    = document.getElementById('tt-pop');
  var ttBar    = document.getElementById('tt-bar');
  var ttLive   = document.getElementById('tt-live');
  var ttTemp   = document.getElementById('tt-temp');
  var ttEvents = document.getElementById('tt-events');
  var active   = null;

  function showTip(e, p) {
    var code = p.id.replace('lan-', '');
    ttName.textContent = p.dataset.name;
    ttCode.textContent = code;
    ttPop.textContent  = p.dataset.pop + ' invånare';
    ttBar.style.width  = Math.max(4, (popOf(p) / maxPop) * 100) + '%';

    var lv = live[code];
    if (lv && (lv.avg_temp_c !== null || lv.police_events_7d > 0)) {
      ttTemp.textContent = lv.avg_temp_c !== null
        ? lv.avg_temp_c.toFixed(1).replace('.', ',') + '°C'
        : '–';
      ttEvents.textContent = lv.police_events_7d + ' händelser';
      ttLive.hidden = false;
    } else {
      ttLive.hidden = true;
    }

    /* reset touch hint state for desktop hover */
    var hint = document.getElementById('tt-touch-hint');
    var cta  = document.getElementById('tt-cta');
    if (hint) hint.hidden = true;
    if (cta)  cta.hidden  = false;

    tooltip.classList.add('visible');
    moveTip(e);
  }

  function moveTip(e) {
    var tw = tooltip.offsetWidth  || 200;
    var th = tooltip.offsetHeight || 110;
    var x  = e.clientX + 18;
    var y  = e.clientY + 18;
    if (x + tw > window.innerWidth  - 12) x = e.clientX - tw - 12;
    if (y + th > window.innerHeight - 12) y = e.clientY - th - 12;
    tooltip.style.left = x + 'px';
    tooltip.style.top  = y + 'px';
  }

  paths.forEach(function (p) {
    p.addEventListener('mouseenter', function (e) {
      if (active) active.classList.remove('active');
      active = p;
      p.classList.add('active');
      showTip(e, p);
    });
    p.addEventListener('mousemove', moveTip);
    p.addEventListener('mouseleave', function () {
      p.classList.remove('active');
      active = null;
      tooltip.classList.remove('visible');
    });
    p.addEventListener('click', function () {
      window.location.href = '/lan/' + p.id.replace('lan-', '');
    });
    p.addEventListener('touchstart', function (e) {
      e.preventDefault();
      var t = e.touches[0];
      if (active === p) {
        /* second tap on same county — navigate */
        window.location.href = '/lan/' + p.id.replace('lan-', '');
        return;
      }
      if (active) active.classList.remove('active');
      active = p;
      p.classList.add('active');
      showTip(t, p);
      /* show touch hint so user knows to tap again */
      var hint = document.getElementById('tt-touch-hint');
      var cta  = document.getElementById('tt-cta');
      if (hint) hint.hidden = false;
      if (cta)  cta.hidden  = true;
    }, { passive: false });
  });

  /* ── Stat count-up ─────────────────────────────────── */
  function countUp(el) {
    var target   = parseFloat(el.dataset.count);
    var decimals = parseInt(el.dataset.decimals || '0', 10);
    var dur      = 1400;
    var start    = null;
    function frame(ts) {
      if (!start) start = ts;
      var t = Math.min((ts - start) / dur, 1);
      var eased = 1 - Math.pow(1 - t, 3);
      var val = target * eased;
      el.textContent = decimals
        ? val.toFixed(decimals).replace('.', ',')
        : Math.round(val).toString();
      if (t < 1) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }
  setTimeout(function () {
    document.querySelectorAll('[data-count]').forEach(countUp);
  }, 500);

  /* ── Ticker: duplicate content for seamless loop ───── */
  var ticker = document.getElementById('ticker');
  ticker.innerHTML += ticker.innerHTML;

  /* ── Cards: scroll reveal + cursor glow ────────────── */
  var cards = Array.prototype.slice.call(document.querySelectorAll('.card'));

  var io = new IntersectionObserver(function (entries) {
    entries.forEach(function (en) {
      if (en.isIntersecting) {
        var i = cards.indexOf(en.target);
        en.target.style.transitionDelay = (i % 4) * 70 + 'ms';
        en.target.classList.add('in');
        io.unobserve(en.target);
      }
    });
  }, { threshold: 0.15 });

  cards.forEach(function (c) {
    io.observe(c);
    c.addEventListener('mousemove', function (e) {
      var r = c.getBoundingClientRect();
      c.style.setProperty('--mx', (e.clientX - r.left) + 'px');
      c.style.setProperty('--my', (e.clientY - r.top) + 'px');
    });
  });
  /* ── Map dimension toggle ──────────────────────────── */
  var rankingsCache = null;

  var dimConfig = {
    population: {
      label: ['Färre', 'Fler invånare'],
      ramp: 'linear-gradient(90deg,#101a2c,#1d4a74)',
      getVal: function (item) { return null; } // uses data-pop (already tinted)
    },
    income: {
      label: ['Lägst inkomst', 'Högst inkomst'],
      ramp: 'linear-gradient(90deg,#0d1a24,#1aaa5e)',
      colorA: '#0d1a24', colorB: '#1aaa5e',
      getVal: function (item) { return item.metrics.income_median; }
    },
    crime: {
      label: ['Lägst brottslighet', 'Högst brottslighet'],
      ramp: 'linear-gradient(90deg,#0f2a1e,#b52020)',
      colorA: '#0f2a1e', colorB: '#b52020',
      getVal: function (item) { return item.metrics.crime_per_100k; }
    },
    education: {
      label: ['Lägst meritvärde', 'Högst meritvärde'],
      ramp: 'linear-gradient(90deg,#0f1535,#6655d4)',
      colorA: '#0f1535', colorB: '#6655d4',
      getVal: function (item) { return item.metrics.school_grade; }
    },
    health: {
      label: ['Kortare livslängd', 'Längre livslängd'],
      ramp: 'linear-gradient(90deg,#1a0f2a,#9a3add)',
      colorA: '#1a0f2a', colorB: '#9a3add',
      getVal: function (item) { return item.metrics.life_expectancy; }
    },
    politics: {
      label: ['Mer vänster', 'Mer höger 2022'],
      ramp: 'linear-gradient(90deg,#c0253f,#1e56b0)',
      getVal: function (item) { return null; }
    }
  };

  /* Interpolate two hex colors at t=[0..1] */
  function lerpColor(a, b, t) {
    var ar = parseInt(a.slice(1,3),16), ag = parseInt(a.slice(3,5),16), ab2 = parseInt(a.slice(5,7),16);
    var br = parseInt(b.slice(1,3),16), bg = parseInt(b.slice(3,5),16), bb2 = parseInt(b.slice(5,7),16);
    var r = Math.round(ar + (br - ar) * t).toString(16).padStart(2,'0');
    var g = Math.round(ag + (bg - ag) * t).toString(16).padStart(2,'0');
    var b2= Math.round(ab2 + (bb2 - ab2) * t).toString(16).padStart(2,'0');
    return '#' + r + g + b2;
  }

  var partyColors = {
    S:'#E8112D', M:'#1B70AE', SD:'#DDCC00', C:'#009933',
    V:'#B5242D', KD:'#231977', L:'#006AB3', MP:'#83C31E', OVR:'#888888'
  };

  function applyDim(dim, rankings) {
    var cfg = dimConfig[dim];
    var legendMin = document.getElementById('legendMin');
    var legendMax = document.getElementById('legendMax');
    var legendRamp = document.getElementById('legendRamp');

    if (legendMin) legendMin.textContent = cfg.label[0];
    if (legendMax) legendMax.textContent = cfg.label[1];
    if (legendRamp && cfg.ramp) {
      legendRamp.style.background = cfg.ramp;
      legendRamp.style.display = '';
    } else if (legendRamp) {
      legendRamp.style.display = 'none';
    }

    if (dim === 'population') {
      /* restore original tint (uses data-pop set by initial choropleth) */
      paths.forEach(function (p) {
        var code = p.id.replace('lan-', '');
        var pop = popOf(p);
        var t = Math.max(0, Math.min(1, (pop - 50000) / (2400000 - 50000)));
        p.style.fill = lerpColor('#101a2c', '#1d4a74', t);
      });
      return;
    }

    /* Build county → value map from rankings */
    var byCode = {};
    rankings.forEach(function (item) { byCode[item.code] = item; });

    if (dim === 'politics') {
      /* Right-bloc (M+SD+KD+L) vs left-bloc (S+V+MP) share per county.
         Normalize across actual min/max so the full color range is used. */
      var polScores = {};
      rankings.forEach(function (item) {
        if (!item.politics_2022 || !item.politics_2022.parties) return;
        var par = item.politics_2022.parties;
        var right = (par.M || 0) + (par.SD || 0) + (par.KD || 0) + (par.L || 0);
        var left  = (par.S  || 0) + (par.V  || 0) + (par.MP || 0);
        polScores[item.code] = right / (right + left || 1);
      });
      var scores = Object.values(polScores);
      var minS = Math.min.apply(null, scores);
      var maxS = Math.max.apply(null, scores);
      var rangeS = maxS - minS || 1;

      paths.forEach(function (p) {
        var code = p.id.replace('lan-', '');
        if (polScores[code] == null) return;
        var t = (polScores[code] - minS) / rangeS;
        /* diverging: left=vivid red, right=vivid blue */
        p.style.fill = lerpColor('#c0253f', '#1e56b0', t);
      });
      if (legendMin) legendMin.textContent = 'Mer vänster';
      if (legendMax) legendMax.textContent = 'Mer höger 2022';
      if (legendRamp) {
        legendRamp.style.background = 'linear-gradient(90deg,#c0253f,#1e56b0)';
        legendRamp.style.display = '';
      }
      return;
    }

    /* Numeric dimensions */
    var vals = rankings.map(function (item) { return cfg.getVal(item); })
                       .filter(function (v) { return v != null; });
    var minV = Math.min.apply(null, vals);
    var maxV = Math.max.apply(null, vals);
    var range = maxV - minV || 1;

    var colorA = cfg.colorA || '#101a2c';
    var colorB = cfg.colorB || '#2dd4a0';

    paths.forEach(function (p) {
      var code = p.id.replace('lan-', '');
      var item = byCode[code];
      if (!item) return;
      var val = cfg.getVal(item);
      if (val == null) { p.style.fill = '#1a2233'; return; }
      var t = (val - minV) / range;
      p.style.fill = lerpColor(colorA, colorB, t);
    });
  }

  function loadAndApplyDim(dim) {
    if (rankingsCache) { applyDim(dim, rankingsCache); return; }
    fetch('/api/v1/lan/rankings')
      .then(function (r) { return r.json(); })
      .then(function (data) {
        rankingsCache = data.data;
        applyDim(dim, rankingsCache);
      })
      .catch(function () { /* rankings not available */ });
  }

  var toggleBtns = Array.prototype.slice.call(document.querySelectorAll('.map-btn'));
  var activeDim = 'population';

  toggleBtns.forEach(function (btn) {
    btn.addEventListener('click', function () {
      toggleBtns.forEach(function (b) { b.classList.remove('active'); });
      btn.classList.add('active');
      activeDim = btn.dataset.dim;
      loadAndApplyDim(activeDim);
    });
  });

  /* Pre-fetch rankings so first click is instant */
  setTimeout(function () { loadAndApplyDim('population'); }, 1200);

})();
